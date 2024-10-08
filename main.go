package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3" // 只导入，作为驱动

	"github.com/hoshinonyaruko/gensokyo-llm/applogic"
	oneclient "github.com/hoshinonyaruko/gensokyo-llm/common/client"
	"github.com/hoshinonyaruko/gensokyo-llm/config"
	"github.com/hoshinonyaruko/gensokyo-llm/controller"
	"github.com/hoshinonyaruko/gensokyo-llm/fmtf"
	"github.com/hoshinonyaruko/gensokyo-llm/hunyuan"
	"github.com/hoshinonyaruko/gensokyo-llm/server"
	"github.com/hoshinonyaruko/gensokyo-llm/template"
	"github.com/hoshinonyaruko/gensokyo-llm/utils"
)

func main() {
	testFlag := flag.Bool("test", false, "Run the test script, test.txt中的是虚拟信息,一行一条")
	ymlPath := flag.String("yml", "", "指定config.yml的路径")
	vFlag := flag.Bool("v", false, "Run ProcessSensitiveWordsV2")
	tidyFlag := flag.Bool("tidy", false, "Run tidylog")
	flag.Parse()

	// 如果用户指定了-yml参数
	configFilePath := "config.yml" // 默认配置文件路径
	if *ymlPath != "" {
		configFilePath = *ymlPath
	}

	// 检查配置文件是否存在
	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		handleMissingConfigFile(ymlPath, configFilePath)
	} else {
		if *ymlPath != "" {
			fmt.Println("配置载入成功:", *ymlPath)
		}
	}

	// 加载配置
	conf, err := config.LoadConfig(configFilePath)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	// 设置配置文件监视器
	go setupConfigWatcher(configFilePath)

	// 日志落地
	if config.GetSavelogs() {
		fmtf.SetEnableFileLog(true)
	}

	// 测试函数
	if *testFlag {
		// 如果启动参数包含 -test，则执行脚本
		err := utils.PostSensitiveMessages()
		if err != nil {
			log.Fatalf("Error running PostSensitiveMessages: %v", err)
		}
		return // 退出程序
	}
	// Deprecated
	secretId := conf.Settings.SecretId
	secretKey := conf.Settings.SecretKey
	fmtf.Printf("secretId:%v\n", secretId)
	fmtf.Printf("secretKey:%v\n", secretKey)
	region := config.Getregion()
	client, err := hunyuan.NewClientWithSecretId(secretId, secretKey, region)
	if err != nil {
		fmtf.Printf("创建hunyuanapi出错:%v", err)
	}

	db, err := sql.Open("sqlite3", "file:mydb.sqlite?cache=shared&mode=rwc")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	app := &applogic.App{DB: db, Client: client}
	// 在启动服务之前确保所有必要的表都已创建
	err = app.EnsureTablesExist()
	if err != nil {
		log.Fatalf("Failed to ensure database tables exist: %v", err)
	}

	if !config.GetStringob11() {
		// 确保user_context表存在
		err = app.EnsureUserContextTableExists()
		if err != nil {
			log.Fatalf("Failed to ensure user_context table exists: %v", err)
		}
	} else {
		// 确保user_context表存在
		err = app.EnsureUserContextTableExistsSP()
		if err != nil {
			log.Fatalf("Failed to ensure user_context table exists: %v", err)
		}
	}

	// 确保向量表存在
	err = app.EnsureEmbeddingsTablesExist()
	if err != nil {
		log.Fatalf("Failed to ensure EmbeddingsTable table exists: %v", err)
	}

	// 确保 QA缓存表 存在
	err = app.EnsureQATableExist()
	if err != nil {
		log.Fatalf("Failed to ensure EmbeddingsTable table exists: %v", err)
	}

	// 加载基于向量的拦截词 即使文本不同 也能按阈值精准拦截
	err = app.EnsureSensitiveWordsTableExists()
	if err != nil {
		log.Fatalf("Failed to ensure SensitiveWordsTable table exists: %v", err)
	}

	if !config.GetStringob11() {
		// 故事模式存档
		err = app.EnsureCustomTableExist()
		if err != nil {
			log.Fatalf("Failed to ensure CustomTableExist table exists: %v", err)
		}
	} else {
		// 故事模式存档
		err = app.EnsureCustomTableExistSP()
		if err != nil {
			log.Fatalf("Failed to ensure CustomTableExist table exists: %v", err)
		}
	}

	// 用户多个记忆表
	err = app.EnsureUserMemoriesTableExists()
	if err != nil {
		log.Fatalf("Failed to ensure UserMemoriesTableExists table exists: %v", err)
	}

	// 加载 拦截词
	err = app.ProcessSensitiveWords()
	if err != nil {
		log.Fatalf("Failed to ProcessSensitiveWords: %v", err)
	}

	apiType := config.GetApiType() // 调用配置包的函数获取API类型

	switch apiType {
	case 0:
		// 如果API类型是0，使用app.chatHandlerHunyuan
		http.HandleFunc("/conversation", app.ChatHandlerHunyuan)
	case 1:
		// 如果API类型是1，使用app.chatHandlerErnie
		// 如果开启function模式 切换function端点
		if !config.GetFunctionMode() {
			http.HandleFunc("/conversation", app.ChatHandlerErnie)
		} else {
			http.HandleFunc("/conversation", app.ChatHandlerErnieFunction)
		}

	case 2:
		// 如果API类型是2，使用app.chatHandlerChatGpt
		http.HandleFunc("/conversation", app.ChatHandlerChatgpt)
	case 3:
		// 如果API类型是3，使用app.chatHandlerRwkv
		http.HandleFunc("/conversation", app.ChatHandlerRwkv)
	case 4:
		// 如果API类型是4，使用app.chatHandlerTyqw
		http.HandleFunc("/conversation", app.ChatHandlerTyqw)
	case 5:
		// 如果API类型是5，使用app.chatHandlerGlm
		http.HandleFunc("/conversation", app.ChatHandlerGlm)
	case 6:
		// 如果API类型是6，使用app.ChatHandlerYuanQi
		http.HandleFunc("/conversation", app.ChatHandlerYuanQi)
	default:
		// 如果是其他值，可以选择一个默认的处理器或者记录一个错误
		log.Printf("Unknown API type: %d", apiType)
	}

	if config.GetAllApi() {
		http.HandleFunc("/conversation_gpt", app.ChatHandlerChatgpt)
		http.HandleFunc("/conversation_hunyuan", app.ChatHandlerHunyuan)
		http.HandleFunc("/conversation_ernie", app.ChatHandlerErnie)
		http.HandleFunc("/conversation_rwkv", app.ChatHandlerRwkv)
		http.HandleFunc("/conversation_tyqw", app.ChatHandlerTyqw)
		http.HandleFunc("/conversation_glm", app.ChatHandlerGlm)
		http.HandleFunc("/conversation_yq", app.ChatHandlerYuanQi)
	}
	if config.GetSelfPath() != "" {
		rateLimiter := server.NewRateLimiter()
		http.HandleFunc("/uploadpic", server.UploadBase64ImageHandler(rateLimiter))
		http.HandleFunc("/uploadrecord", server.UploadBase64RecordHandler(rateLimiter))
		// 设置静态文件服务目录
		// http.FileServer 返回一个处理器，该处理器会将 HTTP 请求
		// 转发到指定的文件或目录（在这里是 "./channel_temp" 目录）
		fileServer := http.FileServer(http.Dir("./channel_temp"))

		// 使用 http.Handle 设置路由
		// "/channel_temp/" 是 URL 路径前缀，所有以此路径前缀开始的请求
		// 都会由 fileServer 处理器处理
		http.Handle("/channel_temp/", http.StripPrefix("/channel_temp/", fileServer))
	}

	// 简易OneApi
	if conf.Settings.OneApi {
		oneclient.Init()
		go func() {
			r := gin.Default()

			r.POST("/v1/chat/completions", func(c *gin.Context) {
				err := controller.RelayTextHelper(c)
				if err != nil {
					c.JSON(err.StatusCode, gin.H{"error": err.Message})
				}
			})

			// 启动服务器并监听配置文件中的端口
			if err := r.Run(fmt.Sprintf(":%d", conf.Settings.OneApiPort)); err != nil {
				fmt.Printf("Failed to start server: %v\n", err)
			}
		}()
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exeDir := filepath.Dir(exePath)
	blacklistPath := filepath.Join(exeDir, "blacklist.txt")

	// 载入黑名单
	if err := utils.LoadBlacklist(blacklistPath); err != nil {
		log.Fatalf("Failed to load blacklist: %v", err)
	}

	// 判断是否设置多个http地址,获取对应关系
	if len(config.GetHttpPaths()) > 0 {
		utils.FetchAndStoreUserIDs()
	}

	// 启动黑名单文件变动监听
	go utils.WatchBlacklist(blacklistPath)

	// 根据-v参数决定是否运行ProcessSensitiveWordsV2
	if *vFlag {
		err := app.ProcessSensitiveWordsV2()
		if err != nil {
			fmtf.Println("Error running ProcessSensitiveWordsV2:", err)
			return
		}
	}

	// 根据-tidy参数决定是否运行utils.Tidylogs()
	if *tidyFlag {
		utils.Tidylogs()
		fmtf.Println("日志整理完毕")
		return
	}

	// 设置路由
	if !config.GetStringob11() {
		http.HandleFunc("/gensokyo", app.GensokyoHandler)
	} else {
		http.HandleFunc("/gensokyo", app.GensokyoHandlerSP)
	}

	var wspath string
	if conf.Settings.WSPath == "nil" {
		wspath = "/"
	} else {
		wspath = "/" + conf.Settings.WSPath
	}
	http.HandleFunc(wspath, func(w http.ResponseWriter, r *http.Request) {
		server.WsHandler(w, r, conf)
	})
	port := config.GetPort()
	portStr := fmtf.Sprintf(":%d", port)
	fmtf.Printf("listening on %v\n", portStr)

	// 设置信号处理
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		fmt.Println("Shutting down server...")
		server.CloseAllConnections()
		os.Exit(0)
	}()

	// 启动HTTP服务器
	log.Fatal(http.ListenAndServe(portStr, nil))
}

func handleMissingConfigFile(ymlPath *string, configFilePath string) {
	if *ymlPath == "" {
		// 用户没有指定-yml参数，按照默认行为处理
		err := os.WriteFile(configFilePath, []byte(template.ConfigTemplate), 0644)
		if err != nil {
			fmt.Println("Error writing config.yml:", err)
			return
		}
		fmt.Println("请配置config.yml然后再次运行.")
		fmt.Print("按下 Enter 继续...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		os.Exit(0)
	} else {
		// 用户指定了-yml参数，但指定的文件不存在
		fmt.Println("指定的配置文件不存在:", *ymlPath)
		os.Exit(0)
	}
}

func setupConfigWatcher(configFilePath string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Error setting up watcher: %v", err)
	}

	// 添加一个100毫秒的Debouncing
	//fileLoader := &config.ConfigFileLoader{EventDelay: 100 * time.Millisecond}

	// Start the goroutine to handle file system events.
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return // Exit if channel is closed.
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					fmt.Println("检测到配置文件变动:", event.Name)
					//fileLoader.LoadConfigF(configFilePath)
					config.LoadConfig(configFilePath)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return // Exit if channel is closed.
				}
				log.Println("Watcher error:", err)
			}
		}
	}()

	// Add the config file to the list of watched files.
	err = watcher.Add(configFilePath)
	if err != nil {
		log.Fatalf("Error adding watcher: %v", err)
	}
}
