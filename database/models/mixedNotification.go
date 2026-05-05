package models

// MixedNotification 定义了基于多指标联合判断的告警规则。
type MixedNotification struct {
	Id               uint        `json:"id,omitempty" gorm:"primaryKey;autoIncrement"`
	Name             string      `json:"name" gorm:"type:varchar(255)"`
	Clients          StringArray `json:"clients" gorm:"type:longtext"`
	CpuThreshold     float32     `json:"cpu_threshold" gorm:"type:decimal(5,2);default:0"`
	RamThreshold     float32     `json:"ram_threshold" gorm:"type:decimal(5,2);default:0"`
	LoadThreshold    float32     `json:"load_threshold" gorm:"type:decimal(5,2);default:0"`
	TrafficThreshold float32     `json:"traffic_threshold" gorm:"type:decimal(8,2);default:0"` // Mbps，按上下行总和计算
	MatchCount       int         `json:"match_count" gorm:"type:int;not null;default:2"`
	Ratio            float32     `json:"ratio" gorm:"type:decimal(5,2);not null;default:0.60"`
	Interval         int         `json:"interval" gorm:"type:int;not null;default:5"` // 监测间隔（分钟）
	LastNotified     LocalTime   `json:"last_notified"`
}
