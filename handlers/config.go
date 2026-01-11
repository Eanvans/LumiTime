package handlers

import "time"

// SMTPConfig holds SMTP-related settings for sending emails.
type SMTPConfig struct {
	Host      string        `mapstructure:"host" json:"host"`
	Port      string        `mapstructure:"port" json:"port"`
	User      string        `mapstructure:"user" json:"user"`
	Pass      string        `mapstructure:"pass" json:"-"`
	From      string        `mapstructure:"from" json:"from"`
	EnableSSL bool          `mapstructure:"enable_ssl" json:"enable_ssl"`
	Timeout   time.Duration `mapstructure:"timeout_ms" json:"timeout_ms"`
}

// RPCConfig holds RPC service configuration
type RPCConfig struct {
	Address        string `mapstructure:"address" json:"address"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds" json:"timeout_seconds"`
}

// GoogleAPIConfig holds Google AI API configuration
type GoogleAPIConfig struct {
	APIKey string `mapstructure:"api_key" json:"-"`
}

type AlibabaAPIConfig struct {
	APIKey string `mapstructure:"api_key" json:"-"`
	Model  string `mapstructure:"model" json:"model"`
}

var smtpCfg = SMTPConfig{}
var rpcCfg = RPCConfig{}
var googleAPICfg = GoogleAPIConfig{}
var alibabaApiCfg = AlibabaAPIConfig{}

// SetSMTPConfig sets the package-level SMTP configuration used by handlers.
func SetSMTPConfig(cfg SMTPConfig) {
	smtpCfg = cfg
}

// GetSMTPConfig returns a copy of the current SMTP configuration.
func GetSMTPConfig() SMTPConfig { return smtpCfg }

// SetRPCConfig sets the package-level RPC configuration
func SetRPCConfig(cfg RPCConfig) {
	rpcCfg = cfg
}

// GetRPCConfig returns a copy of the current RPC configuration
func GetRPCConfig() RPCConfig { return rpcCfg }

// SetGoogleAPIConfig sets the package-level Google API configuration
func SetGoogleAPIConfig(cfg GoogleAPIConfig) {
	googleAPICfg = cfg
}

// GetGoogleAPIConfig returns a copy of the current Google API configuration
func GetGoogleAPIConfig() GoogleAPIConfig { return googleAPICfg }

func SetAlibabaAPIConfig(cfg AlibabaAPIConfig) {
	alibabaApiCfg = cfg
}

func GetAlibabaAPIConfig() AlibabaAPIConfig {
	return alibabaApiCfg
}
