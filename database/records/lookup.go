package records

import (
	"errors"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"gorm.io/gorm"
)

// GetLatestRecordBefore 返回指定时间之前最近的一条记录。
func GetLatestRecordBefore(uuid string, before time.Time) (*models.Record, error) {
	db := dbcore.GetDBInstance()

	var recent models.Record
	var longTerm models.Record
	recentErr := db.Where("client = ? AND time < ?", uuid, before).Order("time DESC").First(&recent).Error
	longTermErr := db.Table("records_long_term").Where("client = ? AND time < ?", uuid, before).Order("time DESC").First(&longTerm).Error

	switch {
	case recentErr == nil && longTermErr == nil:
		if recent.Time.ToTime().After(longTerm.Time.ToTime()) {
			return &recent, nil
		}
		return &longTerm, nil
	case recentErr == nil:
		return &recent, nil
	case longTermErr == nil:
		return &longTerm, nil
	case errors.Is(recentErr, gorm.ErrRecordNotFound) && errors.Is(longTermErr, gorm.ErrRecordNotFound):
		return nil, gorm.ErrRecordNotFound
	case !errors.Is(recentErr, gorm.ErrRecordNotFound):
		return nil, recentErr
	default:
		return nil, longTermErr
	}
}
