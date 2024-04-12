<p align="center">
  <a href="https://www.github.com/hoshinonyaruko/gensokyo-llm">
    <img src="pic/2.jpg" width="200" height="200" alt="gensokyo-llm">
  </a>
</p>

<div align="center">

# gensokyo-llm

_✨ 适用于Gensokyo以及Onebot的大模型数字人一键端 ✨_  

## 特点

支持所有Onebotv11标准框架.

可一键对接[Gensokyo框架](https://gensokyo.bot) 仅需配置反向http地址用于接收信息，正向http地址用于调用发送api

基于sqlite数据库自动维系上下文，对话模式中，使用重置 命令即可重置

可设置system，角色卡,上下文长度,内置多种模型,混元,文心,chatgpt

同时对外提供带有自动上下文的openai原始风味api(经典3参数,id,parent id,messgae)

可作为api运行，也可一键接入QQ频道机器人[QQ机器人开放平台](https://q.qq.com)

可转换gpt的sse类型，递增还是只发新增的sse

并发环境下的sse内存安全，支持维持多用户同时双向sse传输

## 安全性

多重完备安全措施,尽可能保证开发者和llm应用安全.

可设置多轮模拟QA强化角色提示词，可自定义重置回复，安全词回复,第一重安全措施

支持多gsk-llm互联,形成ai-agent类应用,如一个llm为另一个llm整理提示词,审核提示词,第二重安全措施

向量安全词列表,基于向量相似度的敏感拦截词列表,先于文本替换进行,第三重安全措施

AhoCorasick算法实现的超高效文本IN-Out替换规则，可大量替换n个关键词到各自对应的新关键词,第四重安全措施

结果可再次通过百度-腾讯,文本审核接口,第五重安全措施

日志全面记录,命令行参数-test 从test.txt快速跑安全自测脚本,

命令行 -mlog 将当前储存的所有日志进行QA格式化,每日审验,从实际场景提炼新安全规则,不断增加安全性,第六重安全措施

语言过滤,允许llm只接受所指定的语言,自动将繁体转换为简体应用安全规则,在自己擅长的领域进行防守,第七重安全措施

提示词长度限制,用最原始的方式控制安全,阻止恶意用户构造长提示词,第八重安全措施

通过这些方法,打造尽可能安全llm对话机器人。

文本IN-OUT双层替换,可自行实现内部提示词动态替换,修改,更安全强大

基于sqlite设计的向量数据表结构,可使用缓存来省钱.自定义缓存命中率,精准度.

针对高效高性能高QPS场景优化的专门场景应用，没有冗余功能和指令,全面围绕数字人设计.

## 使用方法
使用命令行运行gensokyo-llm可执行程序

配置config.yml 启动，后监听 port 端口 提供/conversation api

支持中间件开发,在gensokyo框架层到gensokyo-llm的http请求之间,可开发中间件实现向量拓展,数据库拓展,动态修改用户问题.

## 接口调用说明

### 终结点

| 属性  | 详情                                   |
| ----- | -------------------------------------- |
| URL   | http://localhost:46230/conversation    |
| 方法  | POST                                   |

### 请求参数

请求体应为JSON格式，包含以下字段：

| 字段名            | 类型   | 描述                           |
| ----------------- | ------ | ------------------------------ |
| `message`         | String | 要发送的消息内容               |
| `conversationId`  | String | 当前对话的唯一标识符           |
| `parentMessageId` | String | 上一条消息的唯一标识符         |

#### 请求示例

```json
{
    "message": "我第一句话说的什么",
    "conversationId": "07710821-ad06-408c-ba60-1a69bf3ca92a",
    "parentMessageId": "73b144d2-a41f-4aeb-b3bb-8624f0e54ba6"
}
```

#### 返回值示例

成功响应将返回状态码 `200` 和一个JSON对象，包含以下字段：

| 字段名            | 类型                    | 描述                        |
| ----------------- | ----------------------- | --------------------------- |
| `response`        | String                  | 接口的响应消息内容          |
| `conversationId`  | String                  | 当前对话的唯一标识符        |
| `messageId`       | String                  | 当前消息的唯一标识符        |
| `details`         | Object                  | 包含额外的使用详情          |
| `usage`           | Object (在 `details` 中) | 使用详情，如令牌计数        |

#### 响应示例

```json
{
    "response": "回答内容",
    "conversationId": "c9b8746d-aa8c-44b3-804a-bb5ad27f5b84",
    "messageId": "36cc9422-da58-47ec-a25e-e8b8eceb47f5",
    "details": {
        "usage": {
            "prompt_tokens": 88,
            "completion_tokens": 2
        }
    }
}
```


## 兼容性
可在各种架构运行
（原生android暂不支持，sqlitev3需要cgo）
由于cgo编译比较复杂，arm平台，或者其他架构，可试图在对应系统架构下，自行本地编译



## 场景支持

API方式调用
QQ频道直接接入

## 约定参数

审核员请求参数

当需要将请求发给另一个 GSK LLM 作为审核员时，应该返回的 JSON 格式如下：

```json
{"result":%s}
```

这里的 `%s` 代表一个将被替换为具体浮点数值的占位符。

气泡生成请求结果

当请求另一个 GSK LLM 生成气泡时，应该返回的 JSON 格式如下：

```json
["","",""]
```

这表示气泡生成的结果是一个包含三个字符串的数组。这个格式用于在返回结果时指明三个不同的气泡，也可以少于或等于3个.