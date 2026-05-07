package notifier

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/komari-monitor/komari/config"
	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/models"
	recordsdb "github.com/komari-monitor/komari/database/records"
	"github.com/komari-monitor/komari/utils/messageSender"
	"gorm.io/gorm"
)

const (
	DailyTrafficSummaryEnabledKey  = "daily_traffic_summary_enabled"
	DailyTrafficSummaryLastSentKey = "daily_traffic_summary_last_sent_date"
)

var beijingLocation = time.FixedZone("UTC+8", 8*60*60)

type DailyTrafficUsage struct {
	Client   models.Client
	Upload   int64
	Download int64
	HasData  bool
}

// MaybeSendDailyTrafficSummary 每分钟调用一次，在北京时间 00:00-00:05 内尝试发送昨日流量汇总。
func MaybeSendDailyTrafficSummary(now time.Time) error {
	enabled, err := config.GetAs[bool](DailyTrafficSummaryEnabledKey, false)
	if err != nil || !enabled {
		return err
	}

	beijingNow := now.In(beijingLocation)
	if beijingNow.Hour() != 0 || beijingNow.Minute() > 5 {
		return nil
	}

	todayKey := beijingNow.Format("2006-01-02")
	lastSent, err := config.GetAs[string](DailyTrafficSummaryLastSentKey, "")
	if err == nil && lastSent == todayKey {
		return nil
	}

	if err := SendDailyTrafficSummary(beijingNow.AddDate(0, 0, -1)); err != nil {
		return err
	}

	return config.Set(DailyTrafficSummaryLastSentKey, todayKey)
}

// SendDailyTrafficSummary 发送指定北京时间自然日的流量汇总。
func SendDailyTrafficSummary(day time.Time) error {
	summaryDay := day.In(beijingLocation)
	start := time.Date(summaryDay.Year(), summaryDay.Month(), summaryDay.Day(), 0, 0, 0, 0, beijingLocation)
	end := start.Add(24 * time.Hour)

	allClients, err := clients.GetAllClientBasicInfo()
	if err != nil {
		return err
	}

	usages := make([]DailyTrafficUsage, 0, len(allClients))
	for _, client := range allClients {
		usage, calcErr := calculateDailyTrafficUsage(client, start, end)
		if calcErr != nil && calcErr != gorm.ErrRecordNotFound {
			return calcErr
		}
		usages = append(usages, usage)
	}

	message := buildDailyTrafficSummaryMessage(start, usages)
	if strings.TrimSpace(message) == "" {
		return nil
	}

	return sendSummaryText(message)
}

func calculateDailyTrafficUsage(client models.Client, start, end time.Time) (DailyTrafficUsage, error) {
	records, err := recordsdb.GetRecordsByClientAndTime(client.UUID, start, end)
	if err != nil {
		return DailyTrafficUsage{}, err
	}

	if len(records) == 0 {
		return DailyTrafficUsage{Client: client}, nil
	}

	sortDailyTrafficRecords(records)

	prev, err := recordsdb.GetLatestRecordBefore(client.UUID, start)
	if err != nil && err != gorm.ErrRecordNotFound {
		return DailyTrafficUsage{}, err
	}

	var lastUp int64
	var lastDown int64
	if prev != nil {
		lastUp = prev.NetTotalUp
		lastDown = prev.NetTotalDown
	} else {
		lastUp = records[0].NetTotalUp
		lastDown = records[0].NetTotalDown
		records = records[1:]
	}

	upValues := make([]int64, 0, len(records))
	downValues := make([]int64, 0, len(records))
	for _, record := range records {
		upValues = append(upValues, record.NetTotalUp)
		downValues = append(downValues, record.NetTotalDown)
	}
	upload := calculateCounterUsage(lastUp, upValues)
	download := calculateCounterUsage(lastDown, downValues)

	return DailyTrafficUsage{
		Client:   client,
		Upload:   upload,
		Download: download,
		HasData:  true,
	}, nil
}

func trafficDelta(previous, current int64) int64 {
	if current >= previous {
		return current - previous
	}
	// 计数器重置时，按当前值重新起算。
	return current
}

func buildDailyTrafficSummaryMessage(dayStart time.Time, usages []DailyTrafficUsage) string {
	sort.Slice(usages, func(i, j int) bool {
		left := usages[i].Upload + usages[i].Download
		right := usages[j].Upload + usages[j].Download
		if left == right {
			return usages[i].Client.Name < usages[j].Client.Name
		}
		return left > right
	})

	lines := []string{
		"📊 <b>每日流量汇总</b>",
		fmt.Sprintf("<b>统计日期：</b><code>%s</code> <i>UTC+8</i>", dayStart.Format("2006-01-02")),
		"",
	}

	for _, usage := range usages {
		name := usage.Client.Name
		if strings.TrimSpace(name) == "" {
			name = usage.Client.UUID
		}

		if !usage.HasData {
			lines = append(lines,
				fmt.Sprintf("<b>◈ %s</b>", name),
				"┊ <i>当天无有效流量数据</i>",
				"",
			)
			continue
		}

		total := usage.Upload + usage.Download
		lines = append(lines,
			fmt.Sprintf("<b>◈ %s</b>", name),
			fmt.Sprintf("┊ ↑ 上传：<code>%s</code>", humanBytes(usage.Upload)),
			fmt.Sprintf("┊ ↓ 下载：<code>%s</code>", humanBytes(usage.Download)),
			fmt.Sprintf("┊ Σ 合计：<code>%s</code>", humanBytes(total)),
			"",
		)
	}

	lines = append(lines, "<i>此消息由系统在北京时间每日凌晨自动汇总发送</i>")
	return strings.Join(lines, "\n")
}

func sendSummaryText(message string) error {
	return messageSender.SendTextMessage(message, "每日流量汇总")
}
