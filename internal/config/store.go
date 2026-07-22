package config

import (
	"fmt"

	"gorm.io/gorm"
)

// AppSettings 是全局 app_config 表的单行配置。
// 适合存放更新频率低，重要的，其他可前端localstorage。
// 增删配置项直接在此 struct 加减字段即可，GORM 自动迁移。
type AppSettings struct {
	ID               uint   `gorm:"column:id;primaryKey;default:1"`
	LastNovelID      int64  `gorm:"column:last_novel_id;default:0"       json:"last_novel_id"`
	SelectedModelKey string `gorm:"column:selected_model_key;default:''"  json:"selected_model_key"`
	ReasoningEffort  string `gorm:"column:reasoning_effort;default:''"    json:"reasoning_effort"`
	ApprovalMode     string `gorm:"column:approval_mode;default:manual"   json:"approval_mode"`
	LastSessionID    string `gorm:"column:last_session_id;default:''"     json:"last_session_id"`
	UserName         string `gorm:"column:user_name;default:''"            json:"user_name"`
	GitName          string `gorm:"column:git_name;default:'Goink'"         json:"git_name"`
	GitEmail         string `gorm:"column:git_email;default:'goink@local'"  json:"git_email"`
	DismissedVersion string `gorm:"column:dismissed_version;default:''"     json:"dismissed_version"`
	CompressionThreshold float64 `gorm:"column:compression_threshold;default:0.7" json:"compression_threshold"`
	PhaseGateEnabled *bool  `gorm:"column:phase_gate_enabled;default:true" json:"phase_gate_enabled"` // 阶段门禁开关，默认开启
	WebDAVPort       int    `gorm:"column:webdav_port;default:12345" json:"webdav_port"`
	WebDAVUser       string `gorm:"column:webdav_user;default:1" json:"webdav_user"`
	WebDAVPass       string `gorm:"column:webdav_pass;default:1" json:"webdav_pass"`
	MinChapterWords  int    `gorm:"column:min_chapter_words;default:2500" json:"min_chapter_words"` // 章节最少字数（get_chapter_list 校验）
	MaxChapterWords  int    `gorm:"column:max_chapter_words;default:4000" json:"max_chapter_words"` // 章节最多字数（get_chapter_list 校验）
	WindowWidth      int    `gorm:"column:window_width;default:0" json:"window_width"`
	WindowHeight     int    `gorm:"column:window_height;default:0" json:"window_height"`
	WindowX          int    `gorm:"column:window_x;default:0" json:"window_x"`
	WindowY          int    `gorm:"column:window_y;default:0" json:"window_y"`
	APIPort          int    `gorm:"column:api_port;default:9323" json:"api_port"`                             // 移动端连接端口
	LogEnabled       bool   `gorm:"column:log_enabled;default:true" json:"log_enabled"`                       // 文件日志开关
	APIToken         string `gorm:"column:api_token;default:''" json:"api_token"`                             // API 认证 token
}

func (AppSettings) TableName() string { return "app_config" }

// LoadSettings 从全局库读取配置（单行）。首次使用时自动创建空行。
func LoadSettings(db *gorm.DB) (*AppSettings, error) {
	// 确保有且只有一行（id=1）
	if err := db.FirstOrCreate(&AppSettings{}, AppSettings{ID: 1}).Error; err != nil {
		return nil, fmt.Errorf("读取应用配置失败: %w", err)
	}

	var s AppSettings
	if err := db.First(&s, 1).Error; err != nil {
		return nil, fmt.Errorf("读取应用配置失败: %w", err)
	}
	return &s, nil
}

// SaveSettings 将配置写回全局库。
func SaveSettings(db *gorm.DB, s *AppSettings) error {
	s.ID = 1 // 确保只更新同一行
	if err := db.Save(s).Error; err != nil {
		return fmt.Errorf("保存应用配置失败: %w", err)
	}
	return nil
}
