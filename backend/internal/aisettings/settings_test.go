package aisettings

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// fakeRepo 是内存仓储：Load 在未保存时返回 (nil, nil)，与真实实现一致，
// 让 Service 走「用初值播种」的分支。
type fakeRepo struct {
	stored  *Stored
	saveErr error
}

func (r *fakeRepo) Load(context.Context) (*Stored, error) {
	if r.stored == nil {
		return nil, nil
	}
	clone := *r.stored
	return &clone, nil
}

func (r *fakeRepo) Save(_ context.Context, s Stored) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	clone := s
	r.stored = &clone
	return nil
}

// fakeEncryptor 做可逆的假加密。刻意用 base64 而不是「加前缀」——加前缀会让密文
// 里仍含明文子串，那样的桩无法验证「库内不存明文」这条断言。
type fakeEncryptor struct{}

func (fakeEncryptor) Encrypt(plaintext []byte) ([]byte, string, error) {
	return []byte("enc:" + base64.StdEncoding.EncodeToString(plaintext)), "v1", nil
}

func (fakeEncryptor) Decrypt(ciphertext []byte, _ string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimPrefix(string(ciphertext), "enc:"))
}

func (fakeEncryptor) ActiveVersion() string { return "v1" }

func newTestService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := &fakeRepo{}
	return NewService(repo, fakeEncryptor{}, Settings{}), repo
}

func baseUpdate() Settings {
	return Settings{
		Enabled:               true,
		Protocol:              ProtocolChatCompletions,
		BaseURL:               "https://api.example.com/v1",
		Model:                 "m",
		APIKey:                "sk-test-1234",
		RequestTimeout:        30,
		MaxOutputTokens:       4096,
		DailyTokenBudget:      100000,
		MaxConcurrentRequests: 2,
	}
}

// 单次输出上限大于日预算时，每一次调用都会因预留额度不足被拒。
// 这种配置必须在保存阶段挡住，否则运行期表现为「AI 一用就失败」。
func TestUpdateRejectsBudgetBelowOutputLimit(t *testing.T) {
	svc, _ := newTestService(t)
	update := baseUpdate()
	update.MaxOutputTokens = 4096
	update.DailyTokenBudget = 1000

	_, err := svc.Update(context.Background(), update, 1)
	if !errors.Is(err, ErrBudgetBelowOutputLimit) {
		t.Fatalf("expected ErrBudgetBelowOutputLimit, got %v", err)
	}
}

// 预算为 0 表示不设上限，此时不应用一致性校验。
func TestUpdateAllowsZeroBudget(t *testing.T) {
	svc, _ := newTestService(t)
	update := baseUpdate()
	update.MaxOutputTokens = 8192
	update.DailyTokenBudget = 0

	saved, err := svc.Update(context.Background(), update, 1)
	if err != nil {
		t.Fatalf("zero budget should disable the cap, got %v", err)
	}
	if saved.MaxOutputTokens != 8192 {
		t.Fatalf("expected max_output_tokens preserved, got %d", saved.MaxOutputTokens)
	}
}

func TestUpdateRejectsOversizedOutputLimit(t *testing.T) {
	svc, _ := newTestService(t)
	update := baseUpdate()
	update.MaxOutputTokens = maxOutputTokensCeiling + 1
	update.DailyTokenBudget = 200000

	_, err := svc.Update(context.Background(), update, 1)
	if !errors.Is(err, ErrOutputLimitTooLarge) {
		t.Fatalf("expected ErrOutputLimitTooLarge, got %v", err)
	}
}

// 默认值曾是 1200，对中文推理型模型不足（实测需 2400~2600 token），
// 会把响应稳定截断在 finish_reason=length。
func TestUpdateDefaultsOutputLimitTo4096(t *testing.T) {
	svc, _ := newTestService(t)
	update := baseUpdate()
	update.MaxOutputTokens = 0

	saved, err := svc.Update(context.Background(), update, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved.MaxOutputTokens != defaultMaxOutputTokens {
		t.Fatalf("expected default %d, got %d", defaultMaxOutputTokens, saved.MaxOutputTokens)
	}
}

// 空密钥 = 保持原值：界面不回显明文，因此不允许「未填写」被理解为「清空」。
func TestUpdateKeepsStoredKeyWhenAPIKeyEmpty(t *testing.T) {
	svc, repo := newTestService(t)
	if _, err := svc.Update(context.Background(), baseUpdate(), 1); err != nil {
		t.Fatalf("seed save failed: %v", err)
	}

	update := baseUpdate()
	update.APIKey = ""
	update.Model = "m2"
	if _, err := svc.Update(context.Background(), update, 1); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	loaded, err := svc.Load(context.Background())
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.APIKey != "sk-test-1234" {
		t.Fatalf("expected stored key preserved, got %q", loaded.APIKey)
	}
	if repo.stored.APIKeyHint != "****1234" {
		t.Fatalf("expected masked last-four hint preserved, got %q", repo.stored.APIKeyHint)
	}
}

// 库内不得出现明文。
func TestUpdateStoresCiphertextOnly(t *testing.T) {
	svc, repo := newTestService(t)
	if _, err := svc.Update(context.Background(), baseUpdate(), 1); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if strings.Contains(string(repo.stored.APIKeyCiphertext), "sk-test-1234") {
		t.Fatalf("stored ciphertext contains the plaintext key")
	}
}

// 开启状态下清除密钥会留下一个必然失败的运行配置，必须拒绝。
func TestUpdateRejectsClearKeyWhileEnabled(t *testing.T) {
	svc, _ := newTestService(t)
	update := baseUpdate()
	update.ClearAPIKey = true

	_, err := svc.Update(context.Background(), update, 1)
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("expected ErrMissingAPIKey, got %v", err)
	}
}

// 关闭状态下允许清除，清除后 Load 不再返回密钥。
func TestUpdateClearsKeyWhenDisabled(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.Update(context.Background(), baseUpdate(), 1); err != nil {
		t.Fatalf("seed save failed: %v", err)
	}

	update := baseUpdate()
	update.Enabled = false
	update.APIKey = ""
	update.ClearAPIKey = true
	if _, err := svc.Update(context.Background(), update, 1); err != nil {
		t.Fatalf("clear failed: %v", err)
	}

	loaded, err := svc.Load(context.Background())
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.APIKey != "" {
		t.Fatalf("expected key cleared, got %q", loaded.APIKey)
	}
}

// 保存必须触发 onChange，否则控制台改完不生效，只能重启进程。
func TestUpdateFiresOnChange(t *testing.T) {
	svc, _ := newTestService(t)
	var got *Settings
	svc.WithOnChange(func(s Settings) { got = &s })

	if _, err := svc.Update(context.Background(), baseUpdate(), 1); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected onChange to fire")
	}
	if got.Model != "m" {
		t.Fatalf("onChange received model %q", got.Model)
	}
}

func TestUpdateRejectsInvalidProtocol(t *testing.T) {
	svc, _ := newTestService(t)
	update := baseUpdate()
	update.Protocol = "grpc"

	if _, err := svc.Update(context.Background(), update, 1); !errors.Is(err, ErrInvalidProtocol) {
		t.Fatalf("expected ErrInvalidProtocol, got %v", err)
	}
}

func TestUpdateRejectsRelativeBaseURL(t *testing.T) {
	svc, _ := newTestService(t)
	update := baseUpdate()
	update.BaseURL = "/v1"

	if _, err := svc.Update(context.Background(), update, 1); !errors.Is(err, ErrInvalidBaseURL) {
		t.Fatalf("expected ErrInvalidBaseURL, got %v", err)
	}
}
