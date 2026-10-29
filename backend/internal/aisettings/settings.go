// Package aisettings 管理工作台侧的「模型接入」配置。
//
// 为什么独立成包而不塞进 internal/config：环境变量是进程启动时一次性读取的，
// 而模型接入必须能在控制台里改完立刻生效。因此把配置拆成两层——
//
//	config     → 启动初值（首次部署/表为空时播种）
//	aisettings → 运行期真值（单行表 + 加密密钥 + 热更新）
//
// 表内单行（id = 1）。API 密钥用与集群 kubeconfig 相同的 AES-256-GCM 加密器加密，
// 接口只回显末四位提示，永不返回明文。
package aisettings

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 协议取值。responses 为 OpenAI Responses API（POST {base}/responses），
// chat_completions 为传统 Chat Completions（POST {base}/chat/completions）。
const (
	ProtocolResponses       = "responses"
	ProtocolChatCompletions = "chat_completions"
)

var (
	ErrInvalidBaseURL = errors.New("base_url must be an absolute http(s) URL")
	ErrInvalidProtocol = errors.New("protocol must be responses or chat_completions")
	ErrMissingModel    = errors.New("model is required when the provider is enabled")
	ErrMissingAPIKey   = errors.New("api_key is required when the provider is enabled")
)

// Settings 是一份完整的接入配置。APIKey 为明文，仅在进程内存与写入路径上短暂存在，
// 不参与 JSON 序列化（见 api 层的 SettingsView）。
type Settings struct {
	Enabled               bool
	Protocol              string
	BaseURL               string
	Model                 string
	APIKey                string // 明文；空表示「不修改」（更新时）或「未配置」
	ClearAPIKey           bool   // 仅在 Update 中生效：显式清除已保存的密钥
	APIKeyKeyVersion      string
	APIKeyHint            string
	RequestTimeout        time.Duration
	MaxOutputTokens       int
	DailyTokenBudget      int
	MaxConcurrentRequests int
	UpdatedAt             time.Time
	UpdatedBy             int64
}

// record 是 ai_model_settings 的 GORM 映射。
type record struct {
	ID                    int16     `gorm:"column:id;primaryKey"`
	Enabled               bool      `gorm:"column:enabled"`
	Protocol              string    `gorm:"column:protocol"`
	BaseURL               string    `gorm:"column:base_url"`
	Model                 string    `gorm:"column:model"`
	APIKeyCiphertext      []byte    `gorm:"column:api_key_ciphertext"`
	APIKeyKeyVersion      *string   `gorm:"column:api_key_key_version"`
	APIKeyHint            *string   `gorm:"column:api_key_hint"`
	RequestTimeoutSeconds int       `gorm:"column:request_timeout_seconds"`
	MaxOutputTokens       int       `gorm:"column:max_output_tokens"`
	DailyTokenBudget      int64     `gorm:"column:daily_token_budget"`
	MaxConcurrentRequests int       `gorm:"column:max_concurrent_requests"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
	UpdatedBy             *int64    `gorm:"column:updated_by"`
}

func (record) TableName() string { return "ai_model_settings" }

// Encryptor 是本包对密钥加解密的最小依赖，由 cluster.Encryptor 实现。
type Encryptor interface {
	Encrypt(plaintext []byte) ([]byte, string, error)
	Decrypt(ciphertext []byte, keyVersion string) ([]byte, error)
	ActiveVersion() string
}

// Repository 负责持久化。Load 在表为空时返回 (nil, nil)，交由 Service 用初值播种。
type Repository interface {
	Load(ctx context.Context) (*Stored, error)
	Save(ctx context.Context, s Stored) error
}

// Stored 是持久化视图：密钥以密文 + 版本 + 末四位提示的形式保存。
type Stored struct {
	Enabled               bool
	Protocol              string
	BaseURL               string
	Model                 string
	APIKeyCiphertext      []byte
	APIKeyKeyVersion      string
	APIKeyHint            string
	RequestTimeout        time.Duration
	MaxOutputTokens       int
	DailyTokenBudget      int
	MaxConcurrentRequests int
	UpdatedBy             int64
}

// GormRepository 是 Repository 的 PostgreSQL 实现。
type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) Load(ctx context.Context) (*Stored, error) {
	var rec record
	err := r.db.WithContext(ctx).Where("id = 1").Take(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load ai settings: %w", err)
	}
	out := &Stored{
		Enabled:               rec.Enabled,
		Protocol:              rec.Protocol,
		BaseURL:               rec.BaseURL,
		Model:                 rec.Model,
		APIKeyCiphertext:      rec.APIKeyCiphertext,
		RequestTimeout:        time.Duration(rec.RequestTimeoutSeconds) * time.Second,
		MaxOutputTokens:       rec.MaxOutputTokens,
		DailyTokenBudget:      int(rec.DailyTokenBudget),
		MaxConcurrentRequests: rec.MaxConcurrentRequests,
	}
	if rec.APIKeyKeyVersion != nil {
		out.APIKeyKeyVersion = *rec.APIKeyKeyVersion
	}
	if rec.APIKeyHint != nil {
		out.APIKeyHint = *rec.APIKeyHint
	}
	if rec.UpdatedBy != nil {
		out.UpdatedBy = *rec.UpdatedBy
	}
	return out, nil
}

func (r *GormRepository) Save(ctx context.Context, s Stored) error {
	rec := record{
		ID:                    1,
		Enabled:               s.Enabled,
		Protocol:              s.Protocol,
		BaseURL:               s.BaseURL,
		Model:                 s.Model,
		APIKeyCiphertext:      s.APIKeyCiphertext,
		RequestTimeoutSeconds: int(s.RequestTimeout / time.Second),
		MaxOutputTokens:       s.MaxOutputTokens,
		DailyTokenBudget:      int64(s.DailyTokenBudget),
		MaxConcurrentRequests: s.MaxConcurrentRequests,
		UpdatedAt:             time.Now().UTC(),
	}
	if s.APIKeyKeyVersion != "" {
		rec.APIKeyKeyVersion = &s.APIKeyKeyVersion
	}
	if s.APIKeyHint != "" {
		rec.APIKeyHint = &s.APIKeyHint
	}
	if s.UpdatedBy != 0 {
		rec.UpdatedBy = &s.UpdatedBy
	}
	// 整行覆盖：id 固定为 1，Save 会 upsert。
	if err := r.db.WithContext(ctx).Save(&rec).Error; err != nil {
		return fmt.Errorf("save ai settings: %w", err)
	}
	return nil
}

// Service 提供读、写与校验，并在写入成功后回调 OnChange 以热更新 aiexplain。
type Service struct {
	repo      Repository
	encryptor Encryptor
	seed      Settings          // 来自环境变量的初值
	onChange  func(Settings)    // 写入成功后的回调（可为 nil）
}

// NewService 构造 Service。seed 是环境变量提供的初值，仅在表为空时使用一次。
func NewService(repo Repository, enc Encryptor, seed Settings) *Service {
	if seed.Protocol == "" {
		seed.Protocol = ProtocolResponses
	}
	return &Service{repo: repo, encryptor: enc, seed: seed}
}

// WithOnChange 注册配置变更回调，用于热更新解释服务。
func (s *Service) WithOnChange(fn func(Settings)) *Service {
	s.onChange = fn
	return s
}

// Load 读取当前生效配置。表为空时返回初值（不落库，等首次保存再写）。
func (s *Service) Load(ctx context.Context) (Settings, error) {
	stored, err := s.repo.Load(ctx)
	if err != nil {
		return Settings{}, err
	}
	if stored == nil {
		return s.seed, nil
	}
	out := Settings{
		Enabled:               stored.Enabled,
		Protocol:              stored.Protocol,
		BaseURL:               stored.BaseURL,
		Model:                 stored.Model,
		APIKeyKeyVersion:      stored.APIKeyKeyVersion,
		APIKeyHint:            stored.APIKeyHint,
		RequestTimeout:        stored.RequestTimeout,
		MaxOutputTokens:       stored.MaxOutputTokens,
		DailyTokenBudget:      stored.DailyTokenBudget,
		MaxConcurrentRequests: stored.MaxConcurrentRequests,
		UpdatedBy:             stored.UpdatedBy,
	}
	if len(stored.APIKeyCiphertext) > 0 {
		plain, derr := s.encryptor.Decrypt(stored.APIKeyCiphertext, stored.APIKeyKeyVersion)
		if derr != nil {
			// 密钥轮换缺版本时不阻断启动：配置仍可见，只是密钥不可用。
			out.APIKey = ""
		} else {
			out.APIKey = string(plain)
		}
	}
	return out, nil
}

// Update 校验并保存。update.APIKey 为空表示「保持原密钥不变」——这样界面可以在
// 不回显密钥的前提下允许改其它字段。
func (s *Service) Update(ctx context.Context, update Settings, actorID int64) (Settings, error) {
	update.Protocol = strings.TrimSpace(update.Protocol)
	if update.Protocol == "" {
		update.Protocol = ProtocolResponses
	}
	if update.Protocol != ProtocolResponses && update.Protocol != ProtocolChatCompletions {
		return Settings{}, ErrInvalidProtocol
	}
	update.BaseURL = strings.TrimRight(strings.TrimSpace(update.BaseURL), "/")
	if update.BaseURL != "" {
		u, err := url.Parse(update.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return Settings{}, ErrInvalidBaseURL
		}
	}
	update.Model = strings.TrimSpace(update.Model)
	if update.Enabled {
		if update.BaseURL == "" {
			return Settings{}, ErrInvalidBaseURL
		}
		if update.Model == "" {
			return Settings{}, ErrMissingModel
		}
	}
	if update.RequestTimeout <= 0 {
		update.RequestTimeout = 30 * time.Second
	}
	if update.MaxOutputTokens <= 0 {
		update.MaxOutputTokens = 1200
	}
	if update.MaxConcurrentRequests <= 0 {
		update.MaxConcurrentRequests = 2
	}

	current, err := s.Load(ctx)
	if err != nil {
		return Settings{}, err
	}

	stored := Stored{
		Enabled:               update.Enabled,
		Protocol:              update.Protocol,
		BaseURL:               update.BaseURL,
		Model:                 update.Model,
		RequestTimeout:        update.RequestTimeout,
		MaxOutputTokens:       update.MaxOutputTokens,
		DailyTokenBudget:      update.DailyTokenBudget,
		MaxConcurrentRequests: update.MaxConcurrentRequests,
		UpdatedBy:             actorID,
		// 未提供新密钥时沿用原密文与提示。
		APIKeyCiphertext: nil,
		APIKeyKeyVersion: current.APIKeyKeyVersion,
		APIKeyHint:       current.APIKeyHint,
	}

	apiKey := strings.TrimSpace(update.APIKey)
	if update.ClearAPIKey {
		// 显式清除：置空密文与提示。用于「不再保存密钥」或更换提供方前的清理。
		// 开启状态下不允许清除——否则会留下一个必然失败的可运行配置。
		if update.Enabled {
			return Settings{}, ErrMissingAPIKey
		}
		stored.APIKeyCiphertext = nil
		stored.APIKeyKeyVersion = ""
		stored.APIKeyHint = ""
		apiKey = ""
	} else if apiKey != "" {
		ct, version, encErr := s.encryptor.Encrypt([]byte(apiKey))
		if encErr != nil {
			return Settings{}, fmt.Errorf("encrypt api key: %w", encErr)
		}
		stored.APIKeyCiphertext = ct
		stored.APIKeyKeyVersion = version
		stored.APIKeyHint = Hint(apiKey)
	} else {
		// 沿用原密文：需要先取回已存的密文。
		prev, lerr := s.repo.Load(ctx)
		if lerr != nil {
			return Settings{}, lerr
		}
		if prev != nil {
			stored.APIKeyCiphertext = prev.APIKeyCiphertext
		}
	}

	if update.Enabled && apiKey == "" && len(stored.APIKeyCiphertext) == 0 {
		return Settings{}, ErrMissingAPIKey
	}

	if err := s.repo.Save(ctx, stored); err != nil {
		return Settings{}, err
	}

	saved, err := s.Load(ctx)
	if err != nil {
		return Settings{}, err
	}
	if s.onChange != nil {
		s.onChange(saved)
	}
	return saved, nil
}

// Hint 返回密钥的末四位提示，形状固定为 "****abcd"，不足以还原密钥。
func Hint(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

// Mask 用于日志：任何情况下都不要打印完整密钥。
func Mask(key string) string {
	if key == "" {
		return ""
	}
	return Hint(key)
}
