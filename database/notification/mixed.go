package notification

import (
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/utils/notifier"
	"gorm.io/gorm"
)

func AddMixedNotification(rule models.MixedNotification) (uint, error) {
	db := dbcore.GetDBInstance()
	if err := db.Create(&rule).Error; err != nil {
		return 0, err
	}

	return rule.Id, ReloadMixedNotificationSchedule()
}

func DeleteMixedNotification(id []uint) error {
	db := dbcore.GetDBInstance()
	result := db.Where("id IN ?", id).Delete(&models.MixedNotification{})
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return ReloadMixedNotificationSchedule()
}

func EditMixedNotification(rules []*models.MixedNotification) error {
	db := dbcore.GetDBInstance()
	for _, rule := range rules {
		result := db.Model(&models.MixedNotification{}).
			Where("id = ?", rule.Id).
			Select(
				"name",
				"clients",
				"cpu_threshold",
				"ram_threshold",
				"load_threshold",
				"traffic_threshold",
				"match_count",
				"ratio",
				"interval",
			).
			Updates(rule)
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
	}

	return ReloadMixedNotificationSchedule()
}

func GetAllMixedNotifications() ([]models.MixedNotification, error) {
	db := dbcore.GetDBInstance()
	var rules []models.MixedNotification
	if err := db.Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func ReloadMixedNotificationSchedule() error {
	rules, err := GetAllMixedNotifications()
	if err != nil {
		return err
	}
	return notifier.ReloadMixedNotificationSchedule(rules)
}
