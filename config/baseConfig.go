package config

type Model struct {
	Model         string
	BaseURL       string
	APIKey        string
	APIType       string // "openai" or "anthropic"
	Stream        bool   //true or false
	ContextWindow int    // 上下文窗口大小
	ShowReasoning bool   `yaml:"show_reasoning"` // 是否显示推理/思考内容
}
type User struct {
	UserID string
}

type Config struct {
	Model Model `yaml:"model"`
	User  User  `yaml:"user"`
}
