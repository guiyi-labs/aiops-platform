-- 模型接入配置：把 AI 相关设置从「只能改环境变量」变成「可在控制台配置」。
--
-- 设计要点：
--   1) 单行表（id = 1 固定），不引入多租户/多配置档案——本平台是单组织部署，
--      多档案会带来「哪份生效」的歧义，收益不足。
--   2) API 密钥不落明文：api_key_ciphertext 与集群 kubeconfig 复用同一套
--      AES-256-GCM 加密器与密钥版本轮换（internal/cluster.NewEncryptor），
--      api_key_key_version 记录加密时所用版本，便于轮换后仍能解密旧值。
--   3) api_key_hint 只存末四位，用于界面回显「已配置 sk-…abcd」，
--      接口永不返回密钥本身。
--   4) protocol 区分 responses / chat_completions 两种请求形态：
--      OpenAI 官方走 /responses，多数兼容端点只实现 /chat/completions。
--      早期实现把协议写死为 responses，导致国内端点无法接入。
--   5) 环境变量仍作为「首次启动的初值」：表为空时由 env 播种，之后以表为准，
--      避免升级后行为突变，也避免运维必须以 SQL 初始化。
CREATE TABLE IF NOT EXISTS ai_model_settings (
    id                       SMALLINT     PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled                  BOOLEAN      NOT NULL DEFAULT FALSE,
    protocol                 VARCHAR(32)  NOT NULL DEFAULT 'responses'
                             CHECK (protocol IN ('responses', 'chat_completions')),
    base_url                 VARCHAR(512) NOT NULL DEFAULT '',
    model                    VARCHAR(128) NOT NULL DEFAULT '',
    api_key_ciphertext       BYTEA,
    api_key_key_version      VARCHAR(16),
    api_key_hint             VARCHAR(16),
    request_timeout_seconds  INTEGER      NOT NULL DEFAULT 30 CHECK (request_timeout_seconds BETWEEN 1 AND 600),
    max_output_tokens        INTEGER      NOT NULL DEFAULT 1200 CHECK (max_output_tokens BETWEEN 1 AND 100000),
    daily_token_budget       BIGINT       NOT NULL DEFAULT 100000 CHECK (daily_token_budget >= 0),
    max_concurrent_requests  INTEGER      NOT NULL DEFAULT 2 CHECK (max_concurrent_requests BETWEEN 1 AND 64),
    updated_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_by               BIGINT
);

COMMENT ON TABLE ai_model_settings IS 'Single-row table: model provider settings editable from the console (M-new).';
COMMENT ON COLUMN ai_model_settings.api_key_ciphertext IS 'AES-256-GCM ciphertext; never returned by the API, only api_key_hint is.';
COMMENT ON COLUMN ai_model_settings.protocol IS 'responses = POST {base_url}/responses; chat_completions = POST {base_url}/chat/completions.';
