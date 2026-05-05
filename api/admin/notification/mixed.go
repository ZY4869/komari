package notification

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/api"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/notification"
)

func AddMixedNotification(c *gin.Context) {
	var req models.MixedNotification
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}

	normalizeMixedNotification(&req)
	if err := validateMixedNotification(req); err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if taskID, err := notification.AddMixedNotification(req); err != nil {
		api.RespondError(c, http.StatusInternalServerError, err.Error())
	} else {
		api.RespondSuccess(c, gin.H{"task_id": taskID})
	}
}

func DeleteMixedNotification(c *gin.Context) {
	var req struct {
		ID []uint `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := notification.DeleteMixedNotification(req.ID); err != nil {
		api.RespondError(c, http.StatusInternalServerError, err.Error())
	} else {
		api.RespondSuccess(c, nil)
	}
}

func EditMixedNotification(c *gin.Context) {
	var req struct {
		Notifications []*models.MixedNotification `json:"notifications" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, "Invalid request data")
		return
	}

	for _, rule := range req.Notifications {
		normalizeMixedNotification(rule)
		if err := validateMixedNotification(*rule); err != nil {
			api.RespondError(c, http.StatusBadRequest, err.Error())
			return
		}
	}

	if err := notification.EditMixedNotification(req.Notifications); err != nil {
		api.RespondError(c, http.StatusInternalServerError, err.Error())
	} else {
		api.RespondSuccess(c, nil)
	}
}

func GetAllMixedNotifications(c *gin.Context) {
	rules, err := notification.GetAllMixedNotifications()
	if err != nil {
		api.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondSuccess(c, rules)
}

func validateMixedNotification(rule models.MixedNotification) error {
	if len(rule.Clients) == 0 {
		return fmt.Errorf("Clients cannot be empty")
	}
	if rule.Interval <= 0 || rule.Interval > 4*60 {
		return fmt.Errorf("Interval must be between 1 and 240 minutes")
	}
	if rule.Ratio <= 0 || rule.Ratio > 1 {
		return fmt.Errorf("Ratio must be between 0 and 1")
	}

	activeMetrics := 0
	if rule.CpuThreshold > 0 {
		activeMetrics++
	}
	if rule.RamThreshold > 0 {
		activeMetrics++
	}
	if rule.LoadThreshold > 0 {
		activeMetrics++
	}
	if rule.TrafficThreshold > 0 {
		activeMetrics++
	}

	if activeMetrics == 0 {
		return fmt.Errorf("At least one threshold must be configured")
	}
	if rule.MatchCount <= 0 {
		return fmt.Errorf("MatchCount must be greater than 0")
	}
	if rule.MatchCount > activeMetrics {
		return fmt.Errorf("MatchCount cannot exceed enabled metrics count (%d)", activeMetrics)
	}

	return nil
}

func normalizeMixedNotification(rule *models.MixedNotification) {
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		rule.Name = "混合告警"
	}
}
