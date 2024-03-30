package config

import (
	"fmt"
	"math/rand"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	instance *Config
	mu       sync.Mutex
)

type Config struct {
	Version  int      `yaml:"version"`
	Settings Settings `yaml:"settings"`
}

type Settings struct {
	SecretId           string   `yaml:"secretId"`
	SecretKey          string   `yaml:"secretKey"`
	Region             string   `yaml:"region"`
	UseSse             bool     `yaml:"useSse"`
	Port               int      `yaml:"port"`
	HttpPath           string   `yaml:"path"`
	SystemPrompt       []string `yaml:"systemPrompt"`
	IPWhiteList        []string `yaml:"iPWhiteList"`
	MaxTokensHunyuan   int      `yaml:"maxTokensHunyuan"`
	ApiType            int      `yaml:"apiType"`
	WenxinAccessToken  string   `yaml:"wenxinAccessToken"`
	WenxinApiPath      string   `yaml:"wenxinApiPath"`
	MaxTokenWenxin     int      `yaml:"maxTokenWenxin"`
	GptModel           string   `yaml:"gptModel"`
	GptApiPath         string   `yaml:"gptApiPath"`
	GptToken           string   `yaml:"gptToken"`
	MaxTokenGpt        int      `yaml:"maxTokenGpt"`
	GptSafeMode        bool     `yaml:"gptSafeMode"`
	GptSseType         int      `yaml:"gptSseType"`
	Groupmessage       bool     `yaml:"groupMessage"`
	SplitByPuntuations int      `yaml:"splitByPuntuations"`
	HunyuanType        int      `yaml:"hunyuanType"`
	FirstQ             []string `yaml:"firstQ"`
	FirstA             []string `yaml:"firstA"`
	SecondQ            []string `yaml:"secondQ"`
	SecondA            []string `yaml:"secondA"`
	ThirdQ             []string `yaml:"thirdQ"`
	ThirdA             []string `yaml:"thirdA"`
	SensitiveMode      bool     `yaml:"sensitiveMode"`
	SensitiveModeType  int      `yaml:"sensitiveModeType"`
	DefaultChangeWord  string   `yaml:"defaultChangeWord"`
}

// LoadConfig 从文件中加载配置并初始化单例配置
func LoadConfig(path string) (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	// 如果单例已经被初始化了，直接返回
	if instance != nil {
		return instance, nil
	}

	configData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	conf := &Config{}
	err = yaml.Unmarshal(configData, conf)
	if err != nil {
		return nil, err
	}

	// 设置单例实例
	instance = conf
	return instance, nil
}

// 获取secretId
func GetsecretId() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.SecretId
	}
	return "0"
}

// 获取secretKey
func GetsecretKey() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.SecretKey
	}
	return "0"
}

// 获取region
func Getregion() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.Region
	}
	return "0"
}

// 获取useSse
func GetuseSse() bool {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.UseSse
	}
	return false
}

// 获取GetPort
func GetPort() int {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.Port
	}
	return 46230
}

// 获取getHttpPath
func GetHttpPath() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.HttpPath
	}
	return "0"
}

// 获取SystemPrompt
func SystemPrompt() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil && len(instance.Settings.SystemPrompt) > 0 {
		prompts := instance.Settings.SystemPrompt
		if len(prompts) == 1 {
			// 如果只有一个成员，直接返回
			return prompts[0]
		} else {
			selectedIndex := rand.Intn(len(prompts))
			selectedPrompt := prompts[selectedIndex]
			fmt.Printf("Selected system prompt: %s\n", selectedPrompt) // 输出你返回的是哪个
			return selectedPrompt
		}
	}
	//如果是nil返回0代表不使用系统提示词
	return "0"
}

// 获取IPWhiteList
func IPWhiteList() []string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.IPWhiteList
	}
	return nil
}

// 获取最大上下文
func GetMaxTokensHunyuan() int {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.MaxTokensHunyuan
	}
	return 4096
}

// 获取Api类型
func GetApiType() int {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.ApiType
	}
	return 0
}

// 获取WenxinAccessToken
func GetWenxinAccessToken() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.WenxinAccessToken
	}
	return "0"
}

// 获取WenxinApiPath
func GetWenxinApiPath() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.WenxinApiPath
	}
	return "0"
}

// 获取GetMaxTokenWenxin
func GetMaxTokenWenxin() int {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.MaxTokenWenxin
	}
	return 0
}

// 获取GptModel
func GetGptModel() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.GptModel
	}
	return "0"
}

// 获取GptApiPath
func GetGptApiPath() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.GptApiPath
	}
	return "0"
}

// 获取GptToken
func GetGptToken() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.GptToken
	}
	return "0"
}

// 获取MaxTokenGpt
func GetMaxTokenGpt() int {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.MaxTokenGpt
	}
	return 0
}

// gpt安全模式
func GetGptSafeMode() bool {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.GptSafeMode
	}
	return false
}

// 获取GptSseType
func GetGptSseType() int {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.GptSseType
	}
	return 0
}

// 是否开启群信息
func GetGroupmessage() bool {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.Groupmessage
	}
	return false
}

// 获取SplitByPuntuations
func GetSplitByPuntuations() int {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.SplitByPuntuations
	}
	return 0
}

// 获取HunyuanType
func GetHunyuanType() int {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.HunyuanType
	}
	return 0
}

// 获取FirstQ
func GetFirstQ() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil && len(instance.Settings.FirstQ) > 0 {
		questions := instance.Settings.FirstQ
		if len(questions) == 1 {
			// 如果只有一个成员，直接返回
			return questions[0]
		} else {
			// 随机选择一个返回
			selectedIndex := rand.Intn(len(questions))
			selectedQuestion := questions[selectedIndex]
			fmt.Printf("Selected first question: %s\n", selectedQuestion) // 输出你返回的是哪个问题
			return selectedQuestion
		}
	}
	// 如果是nil或者空数组，返回空字符串
	return ""
}

// 获取FirstA
func GetFirstA() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil && len(instance.Settings.FirstA) > 0 {
		answers := instance.Settings.FirstA
		if len(answers) == 1 {
			// 如果只有一个成员，直接返回
			return answers[0]
		} else {
			// 随机选择一个返回
			selectedIndex := rand.Intn(len(answers))
			selectedAnswer := answers[selectedIndex]
			fmt.Printf("Selected first answer: %s\n", selectedAnswer) // 输出你返回的是哪个回答
			return selectedAnswer
		}
	}
	// 如果是nil或者空数组，返回空字符串
	return ""
}

// 获取SecondQ
func GetSecondQ() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil && len(instance.Settings.SecondQ) > 0 {
		questions := instance.Settings.SecondQ
		if len(questions) == 1 {
			return questions[0]
		} else {
			selectedIndex := rand.Intn(len(questions))
			selectedQuestion := questions[selectedIndex]
			fmt.Printf("Selected second question: %s\n", selectedQuestion)
			return selectedQuestion
		}
	}
	return ""
}

// 获取SecondA
func GetSecondA() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil && len(instance.Settings.SecondA) > 0 {
		answers := instance.Settings.SecondA
		if len(answers) == 1 {
			return answers[0]
		} else {
			selectedIndex := rand.Intn(len(answers))
			selectedAnswer := answers[selectedIndex]
			fmt.Printf("Selected second answer: %s\n", selectedAnswer)
			return selectedAnswer
		}
	}
	return ""
}

// 获取ThirdQ
func GetThirdQ() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil && len(instance.Settings.ThirdQ) > 0 {
		questions := instance.Settings.ThirdQ
		if len(questions) == 1 {
			return questions[0]
		} else {
			selectedIndex := rand.Intn(len(questions))
			selectedQuestion := questions[selectedIndex]
			fmt.Printf("Selected third question: %s\n", selectedQuestion)
			return selectedQuestion
		}
	}
	return ""
}

// 获取ThirdA
func GetThirdA() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil && len(instance.Settings.ThirdA) > 0 {
		answers := instance.Settings.ThirdA
		if len(answers) == 1 {
			return answers[0]
		} else {
			selectedIndex := rand.Intn(len(answers))
			selectedAnswer := answers[selectedIndex]
			fmt.Printf("Selected third answer: %s\n", selectedAnswer)
			return selectedAnswer
		}
	}
	return ""
}

// 获取DefaultChangeWord
func GetDefaultChangeWord() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.DefaultChangeWord
	}
	return "*"
}

// 是否SensitiveMode
func GetSensitiveMode() bool {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.SensitiveMode
	}
	return false
}

// 获取SensitiveModeType
func GetSensitiveModeType() int {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.SensitiveModeType
	}
	return 0
}
