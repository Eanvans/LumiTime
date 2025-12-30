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

var smtpCfg = SMTPConfig{}

// SetSMTPConfig sets the package-level SMTP configuration used by handlers.
func SetSMTPConfig(cfg SMTPConfig) {
    smtpCfg = cfg
}

// GetSMTPConfig returns a copy of the current SMTP configuration.
func GetSMTPConfig() SMTPConfig { return smtpCfg }
