package main

import (
	"time"

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/aisettings"
)

// buildAIProvider 按配置里的协议构造对应的 Provider。
//
// 两种协议的结构化输出能力等价（都用 json_schema + strict 约束），差别只在请求形态：
//   - responses：POST {base}/responses（OpenAI 官方）
//   - chat_completions：POST {base}/chat/completions（多数兼容端点）
//
// 配置不完整时返回 nil —— aiexplain 在 provider 为 nil 时统一返回 ErrDisabled，
// 这样「开启但没填全」不会变成运行期 500。
func buildAIProvider(settings aisettings.Settings) (aiexplain.Provider, string) {
	if !settings.Enabled || settings.APIKey == "" || settings.BaseURL == "" || settings.Model == "" {
		return nil, ""
	}
	if settings.Protocol == aisettings.ProtocolChatCompletions {
		return aiexplain.NewChatCompletionsProvider(
			settings.BaseURL, settings.APIKey, settings.Model,
			settings.RequestTimeout, settings.MaxOutputTokens,
		), "chat-completions-compatible"
	}
	return aiexplain.NewResponsesProvider(
		settings.BaseURL, settings.APIKey, settings.Model,
		settings.RequestTimeout, settings.MaxOutputTokens,
	), "responses-compatible"
}

// aiServiceConfig 把接入配置翻译成解释服务的运行参数。
// ReservationTTL 取「请求超时 + 1 分钟」：预留必须在一次调用结束后尽快释放，
// 否则预算会被长期占住。
func aiServiceConfig(settings aisettings.Settings, providerName string) aiexplain.ServiceConfig {
	timeout := settings.RequestTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return aiexplain.ServiceConfig{
		// Enabled 与 provider 必须同真同假：只有配置齐全时 provider 才非 nil。
		Enabled:               providerName != "",
		ProviderName:          providerName,
		Model:                 settings.Model,
		DailyTokenBudget:      settings.DailyTokenBudget,
		MaxConcurrentRequests: settings.MaxConcurrentRequests,
		MaxOutputTokens:       settings.MaxOutputTokens,
		ReservationTTL:        timeout + time.Minute,
	}
}
