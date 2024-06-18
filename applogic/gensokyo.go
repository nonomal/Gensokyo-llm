package applogic

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/hoshinonyaruko/gensokyo-llm/acnode"
	"github.com/hoshinonyaruko/gensokyo-llm/config"
	"github.com/hoshinonyaruko/gensokyo-llm/fmtf"
	"github.com/hoshinonyaruko/gensokyo-llm/prompt"
	"github.com/hoshinonyaruko/gensokyo-llm/promptkb"
	"github.com/hoshinonyaruko/gensokyo-llm/structs"
	"github.com/hoshinonyaruko/gensokyo-llm/utils"
)

var newmsgToStringMap = make(map[string]string)
var stringToIndexMap sync.Map
var processMessageMu sync.Mutex
var messages sync.Map

// UserInfo 结构体用于储存用户信息
type UserInfo struct {
	UserID          int64
	GroupID         int64
	RealMessageType string
	MessageType     string
}

// globalMap 用于存储conversationID与UserInfo的映射
var globalMap sync.Map

// RecordStringById 根据id记录一个string
func RecordStringByNewmsg(id, value string) {
	newmsgToStringMap[id] = value
}

// GetStringById 根据newmsg取出对应的string
func GetStringByNewmsg(newmsg string) string {
	if value, exists := newmsgToStringMap[newmsg]; exists {
		return value
	}
	// 如果id不存在，返回空字符串
	return ""
}

// IncrementIndex 为给定的字符串递增索引
func IncrementIndex(s string) int {
	// 尝试从map中获取值，如果不存在则初始化为0
	val, loaded := stringToIndexMap.LoadOrStore(s, 0)
	if !loaded {
		// 如果这是一个新的键，我们现在将其值设置为1
		stringToIndexMap.Store(s, 1)
		return 1
	}

	// 如果已存在，递增索引
	newVal := val.(int) + 1
	stringToIndexMap.Store(s, newVal)
	return newVal
}

// ResetIndex 重置或删除给定字符串的索引
func ResetIndex(s string) {
	// 直接从map中删除指定的键
	stringToIndexMap.Delete(s)
}

// ResetAllIndexes 清空整个索引map
func ResetAllIndexes() {
	// 重新初始化stringToIndexMap，因为sync.Map没有提供清空所有条目的直接方法
	stringToIndexMap = sync.Map{}
}

// checkMessageForHints 检查消息中是否包含给定的提示词
func checkMessageForHints(message string, selfid int64, promptstr string) bool {
	// 从配置中获取提示词数组
	hintWords := config.GetGroupHintWords(promptstr)
	if len(hintWords) == 0 {
		return true // 未设置,直接返回0
	}

	selfidstr := strconv.FormatInt(selfid, 10)
	// 使用[@+selfid+]格式，向提示词数组增加一个成员
	hintWords = append(hintWords, "[@"+selfidstr+"]")

	// 遍历每个提示词，检查它们是否出现在消息中
	for _, hint := range hintWords {
		if strings.Contains(message, hint) {
			return true // 如果消息包含任一提示词，返回true
		}
	}
	// 如果没有找到任何提示词，记录日志并返回false
	fmtf.Println("No hint words found in the message:", message)
	return false
}

func (app *App) GensokyoHandler(w http.ResponseWriter, r *http.Request) {
	// 只处理POST请求
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	// 获取访问者的IP地址
	ip := r.RemoteAddr             // 注意：这可能包含端口号
	ip = strings.Split(ip, ":")[0] // 去除端口号，仅保留IP地址

	// 获取IP白名单
	whiteList := config.IPWhiteList()

	// 检查IP是否在白名单中
	if !utils.Contains(whiteList, ip) {
		http.Error(w, "Access denied", http.StatusInternalServerError)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// 解析请求体到OnebotGroupMessage结构体
	var message structs.OnebotGroupMessage

	err = json.Unmarshal(body, &message)
	if err != nil {
		fmtf.Printf("Error parsing request body: %+v\n", string(body))
		http.Error(w, "Error parsing request body", http.StatusInternalServerError)
		return
	}

	var promptstr string
	// 读取URL参数 "prompt"
	promptstr = r.URL.Query().Get("prompt")
	if promptstr != "" {
		// 使用 prompt 变量进行后续处理
		fmt.Printf("收到prompt参数: %s\n", promptstr)
	}

	// 判断是否是群聊，然后检查触发词
	if message.RealMessageType != "group_private" && message.MessageType != "private" {
		// 去除含2个[[]]的内容
		checkstr := utils.RemoveBracketsContent(message.RawMessage)
		if !checkMessageForHints(checkstr, message.SelfID, promptstr) {
			// 获取概率值
			chance := config.GetGroupHintChance(promptstr)

			// 生成0-100之间的随机数
			randomValue := rand.Intn(100)

			// 比较随机值与配置中的概率
			if randomValue >= chance {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Group message not hint words."))
				return
			} else {
				// 记录日志，表明概率检查通过
				fmt.Printf("Probability check passed: %d%% chance, random value: %d\n", chance, randomValue)
			}
		} else {
			fmt.Printf("checkMessageForHints check passed")
		}
	}

	var CustomRecord *structs.CustomRecord
	if config.GetGroupContext() == 2 && message.MessageType != "private" {
		// 从数据库读取用户的剧情存档
		CustomRecord, err = app.FetchCustomRecord(message.GroupID + message.SelfID)
		if err != nil {
			fmt.Printf("app.FetchCustomRecord 出错: %s\n", err)
		}
	} else {
		// 从数据库读取用户的剧情存档
		CustomRecord, err = app.FetchCustomRecord(message.UserID + message.SelfID)
		if err != nil {
			fmt.Printf("app.FetchCustomRecord 出错: %s\n", err)
		}
	}

	if CustomRecord != nil {
		// 提示词参数
		if CustomRecord.PromptStr != "" {
			// 用数据库读取到的CustomRecord PromptStr去覆盖当前的PromptStr
			promptstr = CustomRecord.PromptStr
			fmt.Printf("刷新prompt参数: %s,newPromptStrStat:%d\n", promptstr, CustomRecord.PromptStrStat-1)
			newPromptStrStat := CustomRecord.PromptStrStat - 1
			// 根据条件区分群和私聊
			if config.GetGroupContext() == 2 && message.MessageType != "private" {
				err = app.InsertCustomTableRecord(message.GroupID+message.SelfID, promptstr, newPromptStrStat)
				if err != nil {
					fmt.Printf("app.InsertCustomTableRecord 出错: %s\n", err)
				}
			} else {
				err = app.InsertCustomTableRecord(message.UserID+message.SelfID, promptstr, newPromptStrStat)
				if err != nil {
					fmt.Printf("app.InsertCustomTableRecord 出错: %s\n", err)
				}
			}
		}

		// MARK: 提示词之间 整体切换Q
		if config.GetGroupContext() == 2 && message.MessageType != "private" {
			app.ProcessPromptMarks(message.GroupID+message.SelfID, message.Message.(string), &promptstr)
		} else {
			app.ProcessPromptMarks(message.UserID+message.SelfID, message.Message.(string), &promptstr)
		}

		// 提示词之间流转 达到信号量
		if CustomRecord.PromptStrStat-1 <= 0 {
			PromptMarks := config.GetPromptMarks(promptstr)
			if len(PromptMarks) != 0 {
				randomIndex := rand.Intn(len(PromptMarks))
				selectedBranch := PromptMarks[randomIndex]
				newPromptStr := selectedBranch.BranchName

				// 刷新新的提示词给用户目前的状态 新的场景应该从1开始
				if config.GetGroupContext() == 2 && message.MessageType != "private" {
					app.InsertCustomTableRecord(message.GroupID+message.SelfID, newPromptStr, 1)
				} else {
					app.InsertCustomTableRecord(message.UserID+message.SelfID, newPromptStr, 1)
				}

				fmt.Printf("流转prompt参数: %s, newPromptStrStat: %d\n", newPromptStr, 1)
				promptstr = newPromptStr
			}
		}
	} else {
		// MARK: 提示词之间 整体切换Q 当用户没有存档时
		if config.GetGroupContext() == 2 && message.MessageType != "private" {
			app.ProcessPromptMarks(message.GroupID+message.SelfID, message.Message.(string), &promptstr)
		} else {
			app.ProcessPromptMarks(message.UserID+message.SelfID, message.Message.(string), &promptstr)
		}

		var newstat int
		if config.GetPromptMarksLength(promptstr) > 1000 {
			newstat = config.GetPromptMarksLength(promptstr)
		} else {
			newstat = 1
		}

		// 初始状态就是 1 设置了1000以上长度的是固有场景,不可切换
		if config.GetGroupContext() == 2 && message.MessageType != "private" {
			err = app.InsertCustomTableRecord(message.GroupID+message.SelfID, promptstr, newstat)
		} else {
			err = app.InsertCustomTableRecord(message.UserID+message.SelfID, promptstr, newstat)
		}

		if err != nil {
			fmt.Printf("app.InsertCustomTableRecord 出错: %s\n", err)
		}
	}

	// 直接从ob11事件获取selfid
	selfid := strconv.FormatInt(message.SelfID, 10)

	// 读取URL参数 "api"
	api := r.URL.Query().Get("api")
	if api != "" {
		// 使用 prompt 变量进行后续处理
		fmt.Printf("收到api参数: %s\n", api)
	}

	// 从URL查询参数中获取skip_lang_check
	skipLangCheckStr := r.URL.Query().Get("skip_lang_check")

	// 默认skipLangCheck为false
	skipLangCheck := false

	if skipLangCheckStr != "" {
		// 尝试将获取的字符串转换为布尔值
		var err error
		skipLangCheck, err = strconv.ParseBool(skipLangCheckStr)
		if err != nil {
			// 如果转换出错，向客户端返回错误消息
			fmt.Fprintf(w, "Invalid skip_lang_check value: %s", skipLangCheckStr)
			return
		}
		fmt.Printf("收到 skip_lang_check 参数: %v\n", skipLangCheck)
	}

	// 打印日志信息，包括prompt参数
	fmtf.Printf("收到onebotv11信息: %+v\n", string(body))

	// 打印消息和其他相关信息
	fmtf.Printf("Received message: %v\n", message.Message)
	fmtf.Printf("Full message details: %+v\n", message)

	// 进行array转换
	// 检查并解析消息类型
	if _, ok := message.Message.(string); !ok {
		// 如果不是字符串，处理消息以转换为字符串,强制转换
		message.Message = ParseMessageContent(message.Message)
	}

	// 判断message.Message的类型
	switch msg := message.Message.(type) {
	case string:
		// message.Message是一个string
		fmtf.Printf("userid:[%v]Received string message: %s\n", message.UserID, msg)

		//是否过滤群信息
		if !config.GetGroupmessage() {
			fmtf.Printf("你设置了不响应群信息：%v", message)
			return
		}

		// 从GetRestoreCommand获取重置指令的列表
		restoreCommands := config.GetRestoreCommand()

		checkResetCommand := msg
		if config.GetIgnoreExtraTips() {
			checkResetCommand = utils.RemoveBracketsContent(checkResetCommand)
		}

		// 去除at自己的 CQ码,如果不是指向自己的,则不响应
		checkResetCommand = utils.RemoveAtTagContentConditionalWithoutAddNick(checkResetCommand, message)

		// 检查checkResetCommand是否在restoreCommands列表中
		isResetCommand := false
		for _, command := range restoreCommands {
			if checkResetCommand == command {
				isResetCommand = true
				break
			}
		}

		if utils.BlacklistIntercept(message, selfid, promptstr) {
			fmtf.Printf("userid:[%v]groupid:[%v]这位用户或群在黑名单中,被拦截", message.UserID, message.GroupID)
			return
		}

		//处理重置指令
		if isResetCommand {
			fmtf.Println("处理重置操作")
			if config.GetGroupContext() == 2 && message.MessageType != "private" {
				app.migrateUserToNewContext(message.GroupID + message.SelfID)
			} else {
				app.migrateUserToNewContext(message.UserID + message.SelfID)
			}
			RestoreResponse := config.GetRandomRestoreResponses()
			if message.RealMessageType == "group_private" || message.MessageType == "private" {
				if !config.GetUsePrivateSSE() {
					utils.SendPrivateMessage(message.UserID, RestoreResponse, selfid, promptstr)
				} else {
					utils.SendSSEPrivateRestoreMessage(message.UserID, RestoreResponse, promptstr)
				}
			} else {
				utils.SendGroupMessage(message.GroupID, message.UserID, RestoreResponse, selfid, promptstr)
			}
			// 处理故事情节的重置
			if config.GetGroupContext() == 2 && message.MessageType != "private" {
				app.deleteCustomRecord(message.GroupID + message.SelfID)
			} else {
				app.deleteCustomRecord(message.UserID + message.SelfID)
			}
			return
		}

		withdrawCommand := config.GetWithdrawCommand()

		// 检查checkResetCommand是否在WithdrawCommand列表中
		iswithdrawCommand := false
		for _, command := range withdrawCommand {
			if checkResetCommand == command {
				iswithdrawCommand = true
				break
			}
		}

		// 处理撤回信息
		if iswithdrawCommand {
			handleWithdrawMessage(message)
			return
		}

		// newmsg 是一个用于缓存和安全判断的临时量
		newmsg := message.Message.(string)
		// 去除注入的提示词
		if config.GetIgnoreExtraTips() {
			newmsg = utils.RemoveBracketsContent(newmsg)
		}

		var (
			vector               []float64
			lastSelectedVectorID int // 用于存储最后选取的相似文本的ID
		)

		// 进行字数拦截
		if config.GetQuestionMaxLenth() != 0 {
			if utils.LengthIntercept(newmsg, message, selfid, promptstr) {
				fmtf.Printf("字数过长,可在questionMaxLenth配置项修改,Q: %v", newmsg)
				// 发送响应
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("question too long"))
				return
			}
		}

		// 进行语言判断拦截 skipLangCheck为false时
		if len(config.GetAllowedLanguages()) > 0 && !skipLangCheck {
			if utils.LanguageIntercept(newmsg, message, selfid, promptstr) {
				fmtf.Printf("不安全!不支持的语言,可在config.yml设置允许的语言,allowedLanguages配置项,Q: %v", newmsg)
				// 发送响应
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("language not support"))
				return
			}
		}

		// 如果使用向量缓存 或者使用 向量安全词
		if config.GetUseCache(promptstr) == 2 || config.GetVectorSensitiveFilter() {
			if config.GetPrintHanming() {
				fmtf.Printf("计算向量的文本: %v", newmsg)
			}
			// 计算文本向量
			vector, err = app.CalculateTextEmbedding(newmsg)
			if err != nil {
				fmtf.Printf("Error calculating text embedding: %v", err)
				// 发送响应
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Error calculating text embedding"))
				return
			}
		}

		// 向量安全词部分,机器人向量安全屏障
		if config.GetVectorSensitiveFilter() {
			ret, retstr, err := app.InterceptSensitiveContent(vector, message, selfid, promptstr)
			if err != nil {
				fmtf.Printf("Error in InterceptSensitiveContent: %v", err)
				// 发送响应
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Error in InterceptSensitiveContent"))
				return
			}
			if ret != 0 {
				fmtf.Printf("sensitive content detected!%v\n", message)
				// 发送响应
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("sensitive content detected![" + retstr + "]"))
				return
			}
		}

		// 缓存省钱部分
		if config.GetUseCache(promptstr) == 2 {
			//fmtf.Printf("计算向量: %v", vector)
			cacheThreshold := config.GetCacheThreshold()
			// 搜索相似文本和对应的ID
			similarTexts, ids, err := app.searchForSingleVector(vector, cacheThreshold)
			if err != nil {
				fmtf.Printf("Error searching for similar texts: %v", err)
				// 发送响应
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Error searching for similar texts"))
				return
			}

			if len(similarTexts) > 0 {
				// 总是获取最相似的文本的ID，不管是否最终使用
				lastSelectedVectorID = ids[0]

				chance := rand.Intn(100)
				// 检查是否满足设定的概率
				if chance < config.GetCacheChance() {
					// 使用最相似的文本的答案
					fmtf.Printf("读取表:%v\n", similarTexts[0])
					responseText, err := app.GetRandomAnswer(similarTexts[0])
					if err == nil {
						fmtf.Printf("缓存命中,Q:%v,A:%v\n", newmsg, responseText)
						//加入上下文
						if app.AddSingleContext(message, responseText) {
							fmtf.Printf("缓存加入上下文成功")
						}
						// 发送响应消息
						if message.RealMessageType == "group_private" || message.MessageType == "private" {
							if !config.GetUsePrivateSSE() {
								utils.SendPrivateMessage(message.UserID, responseText, selfid, promptstr)
							} else {
								utils.SendSSEPrivateMessage(message.UserID, responseText, promptstr)
							}
						} else {
							utils.SendGroupMessage(message.GroupID, message.UserID, responseText, selfid, promptstr)
						}
						// 发送响应
						w.WriteHeader(http.StatusOK)
						w.Write([]byte("Request received and use cache"))
						return // 成功使用缓存答案，提前退出
					} else {
						fmtf.Printf("Error getting random answer: %v", err)

					}
				} else {
					fmtf.Printf("缓存命中，但没有符合概率，继续执行后续代码\n")
					// 注意：这里不需要再生成 lastSelectedVectorID，因为上面已经生成
				}
			} else {
				// 没有找到相似文本，存储新的文本及其向量
				newVectorID, err := app.insertVectorData(newmsg, vector)
				if err != nil {
					fmtf.Printf("Error inserting new vector data: %v", err)
					// 发送响应
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("Error inserting new vector data"))
					return
				}
				lastSelectedVectorID = int(newVectorID) // 存储新插入向量的ID
				fmtf.Printf("没找到缓存,准备储存了lastSelectedVectorID: %v\n", lastSelectedVectorID)
			}

			// 这里继续执行您的逻辑，比如生成新的答案等
			// 注意：根据实际情况调整后续逻辑
		}

		//提示词安全部分
		if config.GetAntiPromptAttackPath() != "" {
			if checkResponseThreshold(newmsg) {
				fmtf.Printf("提示词不安全,过滤:%v", message)
				saveresponse := config.GetRandomSaveResponse()
				if saveresponse != "" {
					if message.RealMessageType == "group_private" || message.MessageType == "private" {
						if !config.GetUsePrivateSSE() {
							utils.SendPrivateMessage(message.UserID, saveresponse, selfid, promptstr)
						} else {
							utils.SendSSEPrivateSafeMessage(message.UserID, saveresponse, promptstr)
						}
					} else {
						utils.SendGroupMessage(message.GroupID, message.UserID, saveresponse, selfid, promptstr)
					}
				}
				// 发送响应
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Request received and not safe"))
				return
			}
		}

		var conversationID, parentMessageID string
		// 请求conversation api 增加当前群/用户上下文
		if config.GetGroupContext() == 2 && message.MessageType != "private" {
			conversationID, parentMessageID, err = app.handleUserContext(message.GroupID + message.SelfID)
		} else {
			conversationID, parentMessageID, err = app.handleUserContext(message.UserID + message.SelfID)
		}

		// 使用map映射conversationID和uid gid的关系
		StoreUserInfo(conversationID, message.UserID, message.GroupID, message.RealMessageType, message.MessageType)

		// 保存记忆
		memoryCommand := config.GetMemoryCommand()

		// 检查checkResetCommand是否在memoryCommand列表中
		ismemoryCommand := false
		for _, command := range memoryCommand {
			if checkResetCommand == command {
				ismemoryCommand = true
				break
			}
		}

		// 处理保存记忆
		if ismemoryCommand {
			app.handleSaveMemory(message, conversationID, parentMessageID, promptstr) // 适配群
			return
		}

		// 记忆列表
		memoryLoadCommand := config.GetMemoryLoadCommand()

		// 检查checkResetCommand是否在memoryLoadCommand列表中或以其为前缀
		ismemoryLoadCommand := false
		isPrefixedMemoryLoadCommand := false // 新增变量用于检测前缀匹配
		for _, command := range memoryLoadCommand {
			if checkResetCommand == command {
				ismemoryLoadCommand = true
				break
			}
			if strings.HasPrefix(checkResetCommand, command) { // 检查前缀
				isPrefixedMemoryLoadCommand = true
			}
		}

		// 处理记忆列表
		if ismemoryLoadCommand {
			app.handleMemoryList(message, promptstr) // 适配群
			return
		}

		// 新增处理载入记忆的逻辑
		if isPrefixedMemoryLoadCommand {
			app.handleLoadMemory(message, checkResetCommand, promptstr) // 适配群
			return
		}

		// 新对话
		newConversationCommand := config.GetNewConversationCommand()

		// 检查checkResetCommand是否在newConversationCommand列表中
		isnewConversationCommand := false
		for _, command := range newConversationCommand {
			if checkResetCommand == command {
				isnewConversationCommand = true
				break
			}
		}

		// 处理新对话
		if isnewConversationCommand {
			app.handleNewConversation(message, conversationID, parentMessageID, promptstr) // 适配群
			return
		}

		//每句话清空上一句话的messageBuilder
		ClearMessage(conversationID)
		fmtf.Printf("conversationID: %s,parentMessageID%s\n", conversationID, parentMessageID)
		if err != nil {
			fmtf.Printf("Error handling user context: %v\n", err)
			// 发送响应
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Error handling user context"))
			return
		}

		// 请求模型使用原文请求,并应用安全策略
		requestmsg := message.Message.(string)

		if config.GetPrintHanming() {
			fmtf.Printf("消息进入替换前:%v", requestmsg)
		}

		// 繁体转换简体 安全策略
		requestmsg, err = utils.ConvertTraditionalToSimplified(requestmsg)
		if err != nil {
			fmtf.Printf("繁体转换简体失败:%v", err)
		}

		// 替换in替换词规则
		if config.GetSensitiveMode() {
			requestmsg = acnode.CheckWordIN(requestmsg)
		}

		// MARK: 对当前的Q进行各种处理

		// 关键词退出部分ExitChoicesQ
		app.ProcessExitChoicesQ(promptstr, &requestmsg, &message, selfid) // 适配群

		// 故事模式规则 应用 PromptChoiceQ
		app.ApplyPromptChoiceQ(promptstr, &requestmsg, &message) // 适配群

		// 故事模式规则 应用 PromptCoverQ
		app.ApplyPromptCoverQ(promptstr, &requestmsg, &message) // 适配群

		// promptstr 随 switchOnQ 变化 切换Q
		app.ApplySwitchOnQ(&promptstr, &requestmsg, &message) // 适配群

		// 概率的添加内容到当前的Q后方
		app.ApplyPromptChanceQ(promptstr, &requestmsg, &message) // 适配群

		// 从数据库读取用户的剧情存档
		var CustomRecord *structs.CustomRecord
		if config.GetGroupContext() == 2 && message.MessageType != "private" {
			CustomRecord, err = app.FetchCustomRecord(message.GroupID + message.SelfID)
			if err != nil {
				fmt.Printf("app.FetchCustomRecord 出错: %s\n", err)
			}
		} else {
			CustomRecord, err = app.FetchCustomRecord(message.UserID + message.SelfID)
			if err != nil {
				fmt.Printf("app.FetchCustomRecord 出错: %s\n", err)
			}
		}

		// 生成场景
		if config.GetEnvType(promptstr) == 1 {
			fmtf.Printf("ai生成背景type=1:%v,当前场景stat:%v,当前promptstr:%v\n", "Q"+newmsg, CustomRecord.PromptStrStat, promptstr)
			PromptMarksLength := config.GetPromptMarksLength(promptstr)
			app.GetAndSendEnv(requestmsg, promptstr, message, selfid, CustomRecord.PromptStrStat, PromptMarksLength)
		}

		// 按提示词区分的细化替换 这里主要不是为了安全和敏感词,而是细化效果,也就没有使用acnode提高效率
		requestmsg = utils.ReplaceTextIn(requestmsg, promptstr)

		// 去除不是针对自己的at CQ码 不响应目标不是自己的at信息
		requestmsg = utils.RemoveAtTagContentConditional(requestmsg, message, promptstr)
		if requestmsg == "" {
			fmtf.Printf("requestmsg is empty")
			// 发送响应
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("requestmsg is empty"))
			return
		}

		if config.GetGroupContext() == 2 && message.MessageType != "private" {
			fmtf.Printf("实际请求conversation端点内容:[%v]%v\n", message.GroupID+message.SelfID, requestmsg)
		} else {
			fmtf.Printf("实际请求conversation端点内容:[%v]%v\n", message.UserID+message.SelfID, requestmsg)
		}

		requestBody, err := json.Marshal(map[string]interface{}{
			"message":         requestmsg,
			"conversationId":  conversationID,
			"parentMessageId": parentMessageID,
			"user_id":         message.UserID,
		})

		if err != nil {
			fmtf.Printf("Error marshalling request: %v\n", err)
			return
		}

		// 构建URL并发送请求到conversation接口
		port := config.GetPort()
		portStr := fmt.Sprintf(":%d", port)

		// 初始化URL，根据api参数动态调整路径
		basePath := "/conversation"
		if api != "" {
			fmtf.Printf("收到api参数: %s\n", api)
			basePath = "/" + api // 动态替换conversation部分为api参数值
		}

		var baseURL string

		if config.GetLotus(promptstr) == "" {
			baseURL = "http://127.0.0.1" + portStr + basePath
		} else {
			baseURL = config.GetLotus(promptstr) + basePath
		}

		// 在加入prompt之前 判断promptstr.yml是否存在
		if !prompt.CheckPromptExistence(promptstr) {
			fmtf.Printf("该请求内容所对应yml文件不存在:[%v]:[%v]\n", requestmsg, promptstr)
			promptstr = ""
		}

		// 使用net/url包来构建和编码URL
		urlParams := url.Values{}
		if promptstr != "" {
			urlParams.Add("prompt", promptstr)
		}

		// 元器和glm会根据userid参数来自动封禁用户
		if config.GetApiType() == 5 || basePath == "/conversation_glm" || config.GetApiType() == 6 || basePath == "/conversation_yq" {
			urlParams.Add("userid", strconv.FormatInt(message.UserID, 10))
		}

		// 将查询参数编码后附加到基本URL上
		fullURL := baseURL
		if len(urlParams) > 0 {
			fullURL += "?" + urlParams.Encode()
		}

		fmtf.Printf("Generated URL:%v\n", fullURL)

		resp, err := http.Post(fullURL, "application/json", bytes.NewBuffer(requestBody))
		if err != nil {
			fmtf.Printf("Error sending request to conversation interface: %v\n", err)
			return
		}

		defer resp.Body.Close()

		var lastMessageID string
		var response string
		var EnhancedAContent string

		if config.GetuseSse(promptstr) == 2 {
			// 处理SSE流式响应
			reader := bufio.NewReader(resp.Body)
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					if err == io.EOF {
						break // 流结束
					}
					fmtf.Printf("Error reading SSE response: %v\n", err)
					return
				}

				// 忽略空行
				if string(line) == "\n" {
					continue
				}

				// 处理接收到的数据
				if !config.GetHideExtraLogs() {
					fmtf.Printf("Received SSE data: %s", string(line))
				}

				// 去除"data: "前缀后进行JSON解析
				jsonData := strings.TrimPrefix(string(line), "data: ")
				var responseData map[string]interface{}
				if err := json.Unmarshal([]byte(jsonData), &responseData); err == nil {
					//接收到最后一条信息
					if id, ok := responseData["messageId"].(string); ok {

						conversationid := responseData["conversationId"].(string)
						// 从conversation对应的sync map取出对应的用户和群号,避免高并发内容发送错乱
						userinfo, _ := GetUserInfo(conversationid)

						lastMessageID = id // 更新lastMessageID
						// 检查是否有未发送的消息部分
						key := utils.GetKey(userinfo.GroupID, userinfo.UserID)
						accumulatedMessageInterface, exists := groupUserMessages.Load(key)
						var accumulatedMessage string
						if exists {
							accumulatedMessage = accumulatedMessageInterface.(string)
						}

						// 提取response字段
						if response, ok = responseData["response"].(string); ok {
							// 获取按照关键词补充的PromptChoiceA
							if config.GetEnhancedQA(promptstr) {
								EnhancedAContent = app.ApplyPromptChoiceA(promptstr, response, &message)
							}
							// 如果accumulatedMessage是response的子串，则提取新的部分并发送
							if exists && strings.HasPrefix(response, accumulatedMessage) {
								newPart := response[len(accumulatedMessage):]
								if newPart != "" {
									fmtf.Printf("A完整信息: %s,已发送信息:%s 新部分:%s\n", response, accumulatedMessage, newPart)
									// 判断消息类型，如果是私人消息或私有群消息，发送私人消息；否则，根据配置决定是否发送群消息
									if userinfo.RealMessageType == "group_private" || userinfo.MessageType == "private" {
										if !config.GetUsePrivateSSE() {
											utils.SendPrivateMessage(userinfo.UserID, newPart, selfid, promptstr)
										} else {
											//判断是否最后一条
											var state int
											if EnhancedAContent == "" {
												state = 11
											} else {
												state = 1
											}
											messageSSE := structs.InterfaceBody{
												Content: newPart,
												State:   state,
											}
											utils.SendPrivateMessageSSE(userinfo.UserID, messageSSE, promptstr)
										}
									} else {
										// 这里发送的是newPart api最后补充的部分
										if !config.GetMdPromptKeyboardAtGroup() {
											// 如果没有 EnhancedAContent
											if EnhancedAContent == "" {
												utils.SendGroupMessage(userinfo.GroupID, userinfo.UserID, newPart, selfid, promptstr)
											} else {
												utils.SendGroupMessage(userinfo.GroupID, userinfo.UserID, newPart+EnhancedAContent, selfid, promptstr)
											}
										} else {
											// 如果没有 EnhancedAContent
											if EnhancedAContent == "" {
												go utils.SendGroupMessageMdPromptKeyboard(userinfo.GroupID, userinfo.UserID, newPart, selfid, newmsg, response, promptstr)
											} else {
												go utils.SendGroupMessageMdPromptKeyboard(userinfo.GroupID, userinfo.UserID, newPart+EnhancedAContent, selfid, newmsg, response, promptstr)
											}
										}
									}
								} else {
									// 流的最后一次是完整结束的
									fmtf.Printf("A完整信息: %s(sse完整结束)\n", response)
								}

							} else if response != "" {
								// 如果accumulatedMessage不存在或不是子串，print
								fmtf.Printf("B完整信息: %s,已发送信息:%s", response, accumulatedMessage)
								if accumulatedMessage == "" {
									// 判断消息类型，如果是私人消息或私有群消息，发送私人消息；否则，根据配置决定是否发送群消息
									if userinfo.RealMessageType == "group_private" || userinfo.MessageType == "private" {
										if !config.GetUsePrivateSSE() {
											// 如果没有 EnhancedAContent
											if EnhancedAContent == "" {
												utils.SendPrivateMessage(userinfo.UserID, response, selfid, promptstr)
											} else {
												utils.SendPrivateMessage(userinfo.UserID, response+EnhancedAContent, selfid, promptstr)
											}
										} else {
											//判断是否最后一条
											var state int
											if EnhancedAContent == "" {
												state = 11 //准备结束 下一个就是20
											} else {
												state = 1 //下一个是11 由末尾补充负责
											}
											messageSSE := structs.InterfaceBody{
												Content: response,
												State:   state,
											}
											utils.SendPrivateMessageSSE(userinfo.UserID, messageSSE, promptstr)
										}
									} else {
										if !config.GetMdPromptKeyboardAtGroup() {
											// 如果没有 EnhancedAContent
											if EnhancedAContent == "" {
												utils.SendGroupMessage(userinfo.GroupID, userinfo.UserID, response, selfid, promptstr)
											} else {
												utils.SendGroupMessage(userinfo.GroupID, userinfo.UserID, response+EnhancedAContent, selfid, promptstr)
											}
										} else {
											// 如果没有 EnhancedAContent
											if EnhancedAContent == "" {
												go utils.SendGroupMessageMdPromptKeyboard(userinfo.GroupID, userinfo.UserID, response, selfid, newmsg, response, promptstr)
											} else {
												go utils.SendGroupMessageMdPromptKeyboard(userinfo.GroupID, userinfo.UserID, response+EnhancedAContent, selfid, newmsg, response, promptstr)
											}
										}

									}
								}
							}
							// 提示词 整体切换A
							app.ProcessPromptMarks(userinfo.UserID, response, &promptstr)
							// 清空之前加入缓存
							// 缓存省钱部分 这里默认不被覆盖,如果主配置开了缓存,始终缓存.
							if config.GetUseCache() == 2 {
								if response != "" {
									fmtf.Printf("缓存了Q:%v,A:%v,向量ID:%v", newmsg, response, lastSelectedVectorID)
									app.InsertQAEntry(newmsg, response, lastSelectedVectorID)
								} else {
									fmtf.Printf("缓存Q:%v时遇到问题,A为空,检查api是否存在问题", newmsg)
								}
							}

							// 清空key的值
							groupUserMessages.Store(key, "")
						}
					} else {
						//发送信息
						if !config.GetHideExtraLogs() {
							fmtf.Printf("收到流数据,切割并发送信息: %s", string(line))
						}
						splitAndSendMessages(string(line), newmsg, selfid, promptstr)
					}
				}
			}

			// 在流的末尾发送补充的A 因为是SSE
			if EnhancedAContent != "" {
				if message.RealMessageType == "group_private" || message.MessageType == "private" {
					if config.GetUsePrivateSSE() {
						messageSSE := structs.InterfaceBody{
							Content: EnhancedAContent,
							State:   11,
						}
						utils.SendPrivateMessageSSE(message.UserID, messageSSE, promptstr)
					}
				}
			}

			// 在SSE流结束后更新用户上下文 在这里调用gensokyo流式接口的最后一步 插推荐气泡
			if lastMessageID != "" {
				fmtf.Printf("lastMessageID: %s\n", lastMessageID)
				if config.GetGroupContext() == 2 && message.MessageType != "private" {
					err := app.updateUserContext(message.GroupID+message.SelfID, lastMessageID)
					if err != nil {
						fmtf.Printf("Error updating user context: %v\n", err)
					}
				} else {
					err := app.updateUserContext(message.UserID+message.SelfID, lastMessageID)
					if err != nil {
						fmtf.Printf("Error updating user context: %v\n", err)
					}
				}

				if message.RealMessageType == "group_private" || message.MessageType == "private" {
					if config.GetUsePrivateSSE() {

						// 发气泡和按钮
						var promptkeyboard []string
						if !config.GetUseAIPromptkeyboard() {
							promptkeyboard = config.GetPromptkeyboard()
						} else {
							fmtf.Printf("ai生成气泡:%v", "Q"+newmsg+"A"+response)
							promptkeyboard = promptkb.GetPromptKeyboardAI("Q"+newmsg+"A"+response, promptstr)
						}

						// 使用acnode.CheckWordOUT()过滤promptkeyboard中的每个字符串
						for i, item := range promptkeyboard {
							promptkeyboard[i] = acnode.CheckWordOUT(item)
						}

						// 添加第四个气泡
						if config.GetNo4Promptkeyboard() {
							// 合并所有命令到一个数组
							var allCommands []string

							// 获取并添加RestoreResponses
							RestoreResponses := config.GetRestoreCommand()
							allCommands = append(allCommands, RestoreResponses...)

							// 获取并添加memoryLoadCommand
							memoryLoadCommand := config.GetMemoryLoadCommand()
							allCommands = append(allCommands, memoryLoadCommand...)

							// 获取并添加memoryCommand
							memoryCommand := config.GetMemoryCommand()
							allCommands = append(allCommands, memoryCommand...)

							// 获取并添加newConversationCommand
							newConversationCommand := config.GetNewConversationCommand()
							allCommands = append(allCommands, newConversationCommand...)

							// 检查合并后的命令数组长度
							if len(allCommands) > 0 {
								// 随机选择一个命令
								selectedCommand := allCommands[rand.Intn(len(allCommands))]

								// 在promptkeyboard的末尾添加选中的命令
								if len(promptkeyboard) > 0 {
									promptkeyboard = append(promptkeyboard, selectedCommand)
								} else {
									// 如果promptkeyboard为空，我们也应当初始化它，并添加选中的命令
									promptkeyboard = []string{selectedCommand}
								}
							}
						}

						//最后一条了
						messageSSE := structs.InterfaceBody{
							Content:        " ",
							State:          20,
							PromptKeyboard: promptkeyboard,
						}
						utils.SendPrivateMessageSSE(message.UserID, messageSSE, promptstr)
						ResetIndex(newmsg)
					}
				}

			}
		} else {
			// 处理常规响应
			responseBody, err := io.ReadAll(resp.Body)
			if err != nil {
				fmtf.Printf("Error reading response body: %v\n", err)
				return
			}
			fmtf.Printf("Response from conversation interface: %s\n", string(responseBody))

			// 使用map解析响应数据以获取response字段和messageId
			var responseData map[string]interface{}
			if err := json.Unmarshal(responseBody, &responseData); err != nil {
				fmtf.Printf("Error unmarshalling response data: %v\n", err)
				return
			}
			var ok bool
			// 使用提取的response内容发送消息
			if response, ok = responseData["response"].(string); ok && response != "" {
				// 判断消息类型，如果是私人消息或私有群消息，发送私人消息；否则，根据配置决定是否发送群消息
				if message.RealMessageType == "group_private" || message.MessageType == "private" {
					utils.SendPrivateMessage(message.UserID, response, selfid, promptstr)
				} else {
					utils.SendGroupMessage(message.GroupID, message.UserID, response, selfid, promptstr)
				}
			}

			// 更新用户上下文
			if messageId, ok := responseData["messageId"].(string); ok {
				if config.GetGroupContext() == 2 && message.MessageType != "private" {
					err := app.updateUserContext(message.GroupID+message.SelfID, messageId)
					if err != nil {
						fmtf.Printf("Error updating user context: %v\n", err)
					}
				} else {
					err := app.updateUserContext(message.UserID+message.SelfID, messageId)
					if err != nil {
						fmtf.Printf("Error updating user context: %v\n", err)
					}
				}

			}
		}

		// OUT规则不仅对实际发送api生效,也对http结果生效
		if config.GetSensitiveModeType() == 1 {
			response = acnode.CheckWordOUT(response)
		}

		// 发送响应
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Request received and processed Q:" + newmsg + " A:" + response))

		if response == "" {
			return
		}

		// 关键词退出部分A
		app.ProcessExitChoicesA(promptstr, &requestmsg, &message, selfid)

		// 这里的代码也会继续运行,不会影响本次请求的返回值
		// promptstr 随 switchOnA 变化 切换A
		app.ApplySwitchOnA(&promptstr, &requestmsg, &message)

		// 生成场景
		if config.GetEnvType() == 2 {
			fmtf.Printf("ai生成背景type=2:%v,当前场景stat:%v\n", "Q"+newmsg+"A"+response, CustomRecord.PromptStrStat)
			PromptMarksLength := config.GetPromptMarksLength(promptstr)
			app.GetAndSendEnv("Q"+newmsg+"A"+response, promptstr, message, selfid, CustomRecord.PromptStrStat, PromptMarksLength)
		}
	case map[string]interface{}:
		// message.Message是一个map[string]interface{}
		// 理论上不应该执行到这里，因为我们已确保它是字符串
		fmtf.Println("Received map message, handling not implemented yet")
		// 处理map类型消息的逻辑（TODO）

	default:
		// message.Message是一个未知类型
		// 理论上不应该执行到这里，因为我们已确保它是字符串
		fmtf.Printf("Received message of unexpected type: %T\n", msg)
		return
	}

}

func splitAndSendMessages(line string, newmesssage string, selfid string, promptstr string) {
	// 提取JSON部分
	dataPrefix := "data: "
	jsonStr := strings.TrimPrefix(line, dataPrefix)

	// 解析JSON数据
	var sseData struct {
		Response       string `json:"response"`
		ConversationId string `json:"conversationId"`
	}
	err := json.Unmarshal([]byte(jsonStr), &sseData)
	if err != nil {
		fmtf.Printf("Error unmarshalling SSE data: %v\n", err)
		return
	}

	if sseData.Response != "\n\n" {
		// 处理提取出的信息
		processMessage(sseData.Response, sseData.ConversationId, newmesssage, selfid, promptstr)
	} else {
		fmtf.Printf("忽略llm末尾的换行符")
	}
}

func processMessage(response string, conversationid string, newmesssage string, selfid string, promptstr string) {
	// 从conversation对应的sync map取出对应的用户和群号,避免高并发内容发送错乱
	userinfo, _ := GetUserInfo(conversationid)
	key := utils.GetKey(userinfo.GroupID, userinfo.UserID)

	// 定义中文全角和英文标点符号
	punctuations := []rune{'。', '！', '？', '，', ',', '.', '!', '?', '~'}

	for _, char := range response {
		AppendRune(conversationid, char)
		if utils.ContainsRune(punctuations, char, userinfo.GroupID, promptstr) {
			// 达到标点符号，发送累积的整个消息
			if GetMessageLength(conversationid) > 0 {
				accumulatedMessage, _ := GetCurrentMessage(conversationid)
				// 锁定
				processMessageMu.Lock()
				// 从sync.map读取当前的value
				valueInterface, _ := groupUserMessages.Load(key)
				value, _ := valueInterface.(string)
				// 添加当前messageBuilder中的新内容
				value += accumulatedMessage
				// 储存新的内容到sync.map
				groupUserMessages.Store(key, value)
				processMessageMu.Unlock() // 完成更新后时解锁

				// 判断消息类型，如果是私人消息或私有群消息，发送私人消息；否则，根据配置决定是否发送群消息
				if userinfo.RealMessageType == "group_private" || userinfo.MessageType == "private" {
					if !config.GetUsePrivateSSE() {
						utils.SendPrivateMessage(userinfo.UserID, accumulatedMessage, selfid, promptstr)
					} else {
						if IncrementIndex(newmesssage) == 1 {
							//第一条信息
							//取出当前信息作为按钮回调
							//CallbackData := GetStringById(lastMessageID)
							uerid := strconv.FormatInt(userinfo.UserID, 10)
							messageSSE := structs.InterfaceBody{
								Content:      accumulatedMessage,
								State:        1,
								ActionButton: 10,
								CallbackData: uerid,
							}
							utils.SendPrivateMessageSSE(userinfo.UserID, messageSSE, promptstr)
						} else {
							//SSE的前半部分
							messageSSE := structs.InterfaceBody{
								Content: accumulatedMessage,
								State:   1,
							}
							utils.SendPrivateMessageSSE(userinfo.UserID, messageSSE, promptstr)
						}
					}
				} else {
					utils.SendGroupMessage(userinfo.GroupID, userinfo.UserID, accumulatedMessage, selfid, promptstr)
				}

				ClearMessage(conversationid)
			}
		}
	}
}

// 处理撤回信息的函数
func handleWithdrawMessage(message structs.OnebotGroupMessage) {
	fmt.Println("处理撤回操作")
	var id int64

	// 根据消息类型决定使用哪个ID
	switch message.RealMessageType {
	case "group_private", "guild_private":
		id = message.UserID
	case "group", "guild":
		id = message.GroupID
	case "interaction":
		id = message.GroupID
	default:
		fmt.Println("Unsupported message type for withdrawal:", message.RealMessageType)
		return
	}

	// 调用DeleteLatestMessage函数
	err := utils.DeleteLatestMessage(message.RealMessageType, id, message.UserID)
	if err != nil {
		fmt.Println("Error deleting latest message:", err)
		return
	}
}

// StoreUserInfo 用于存储用户信息到全局 map
func StoreUserInfo(conversationID string, userID int64, groupID int64, realMessageType string, messageType string) {
	userInfo := UserInfo{
		UserID:          userID,
		GroupID:         groupID,
		RealMessageType: realMessageType,
		MessageType:     messageType,
	}
	globalMap.Store(conversationID, userInfo)
}

// GetUserInfo 根据conversationID获取用户信息
func GetUserInfo(conversationID string) (UserInfo, bool) {
	value, ok := globalMap.Load(conversationID)
	if ok {
		return value.(UserInfo), true
	}
	return UserInfo{}, false
}

func AppendRune(conversationID string, char rune) {
	value, _ := messages.LoadOrStore(conversationID, "")
	// 追加字符到现有字符串
	updatedValue := value.(string) + string(char)
	messages.Store(conversationID, updatedValue)
}

func GetCurrentMessage(conversationID string) (string, bool) {
	value, ok := messages.Load(conversationID)
	if ok {
		return value.(string), true
	}
	return "", false
}

func ClearMessage(conversationID string) {
	messages.Delete(conversationID)
}

func GetMessageLength(conversationID string) int {
	value, ok := messages.Load(conversationID)
	if ok {
		// 断言字符串，返回长度
		return len(value.(string))
	}
	// 如果没有找到对应的值，返回0
	return 0
}
