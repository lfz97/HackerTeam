package config

type Model struct {
	Model                       string `yaml:"model"`
	BaseURL                     string `yaml:"baseurl"`
	APIKey                      string `yaml:"apikey"`
	APIType                     string `yaml:"apitype"`                     // "openai" or "anthropic"
	AnthropicAuthHeaderTransfer bool   `yaml:"anthropicAuthHeaderTransfer"` //如果true，那么通过Authorization: Bearer认证，如果为false，通过X-Api-Key认证
	Stream                      bool   `yaml:"stream"`                      //true or false
	ContextWindow               int    `yaml:"contextwindow"`               // 上下文窗口大小
	ShowReasoning               bool   `yaml:"show_reasoning"`              // 是否显示推理/思考内容
}
type User struct {
	UserID string `yaml:"userid"`
}

type Config struct {
	Model Model `yaml:"model"`
	User  User  `yaml:"user"`
}
