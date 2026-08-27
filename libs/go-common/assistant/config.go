package assistant

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Enabled, AIEnabled, TelegramEnabled, DevAuthEnabled              bool
	DeepSeekAPIKey, DeepSeekBaseURL, DeepSeekModel, TelegramBotToken string
	BatchSize                                                        int
	Timeout                                                          time.Duration
}

func LoadConfig() Config {
	return Config{
		Enabled:          envBool("ASSISTANT_ENABLED", false),
		AIEnabled:        envBool("ASSISTANT_AI_ENABLED", false),
		TelegramEnabled:  envBool("ASSISTANT_TELEGRAM_ENABLED", false),
		DevAuthEnabled:   envBool("ASSISTANT_DEV_AUTH_ENABLED", false),
		DeepSeekAPIKey:   strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")),
		DeepSeekBaseURL:  strings.TrimSpace(os.Getenv("DEEPSEEK_BASE_URL")),
		DeepSeekModel:    strings.TrimSpace(os.Getenv("DEEPSEEK_MODEL")),
		TelegramBotToken: strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		BatchSize:        envInt("ASSISTANT_BATCH_SIZE", 25, 1, 100),
		Timeout:          time.Duration(envInt("ASSISTANT_TIMEOUT_SEC", 90, 10, 300)) * time.Second,
	}
}
func envBool(key string, fallback bool) bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	return value
}
func envInt(key string, fallback, min, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value < min || value > max {
		return fallback
	}
	return value
}
