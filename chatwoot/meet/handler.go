package meet

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wordgate/qtoolkit/chatwoot/calcom"
	"github.com/wordgate/qtoolkit/slack"
)

//go:embed embed/meet.html
var meetFS embed.FS

var meetTemplate *template.Template

func init() {
	meetTemplate = template.Must(template.ParseFS(meetFS, "embed/meet.html"))
}

// meetPageData is the data injected into meet.html via a JSON blob.
type meetPageData struct {
	DataJSON template.JS // JSON blob injected into <script type="application/json">
}

// meetData is serialized to JSON and injected into the template.
type meetData struct {
	ServerURL   string `json:"serverURL,omitempty"`
	Token       string `json:"token,omitempty"`
	ScheduledAt string `json:"scheduledAt,omitempty"`
	Role        string `json:"role,omitempty"`
	Status      string `json:"status,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Mount registers all meet routes on the given router.
func Mount(r gin.IRouter, path string, reply ReplyFunc) {
	ensureInitialized()

	r.POST(path+"/webhook/calcom", handleCalcomWebhook(reply))
	r.GET(path+"/:token", handleMeetPage())
}

func handleCalcomWebhook(reply ReplyFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		event, err := calcom.ParseWebhook(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()
		cfg := getConfig()
		if cfg == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "meet: not configured"})
			return
		}

		switch event.TriggerEvent {
		case "BOOKING_CREATED":
			handleBookingCreated(ctx, cfg, event, reply, c)
		case "BOOKING_CANCELLED":
			handleBookingCancelled(ctx, cfg, event, reply, c)
		case "BOOKING_RESCHEDULED":
			handleBookingRescheduled(ctx, cfg, event, reply, c)
		default:
			c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		}
	}
}

func handleBookingCreated(ctx context.Context, cfg *Config, event *calcom.Event, reply ReplyFunc, c *gin.Context) {
	id, err := generateID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	customerToken, err := generateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	agentToken, err := generateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	conversationID, _ := strconv.Atoi(event.Metadata["conversation_id"])
	inboxID, _ := strconv.Atoi(event.Metadata["inbox_id"])

	customerName := ""
	customerEmail := ""
	if len(event.Booking.Attendees) > 0 {
		customerName = event.Booking.Attendees[0].Name
		customerEmail = event.Booking.Attendees[0].Email
	}

	duration := event.Booking.EndTime.Sub(event.Booking.StartTime)

	schedule := &Schedule{
		ID:              id,
		CalcomBookingID: event.Booking.ID,
		AgentEmail:      event.Booking.Organizer.Email,
		CustomerName:    customerName,
		CustomerEmail:   customerEmail,
		ConversationID:  conversationID,
		InboxID:         inboxID,
		ScheduledAt:     event.Booking.StartTime,
		Duration:        duration.String(),
		RoomName:        "meet-" + id,
		CustomerToken:   customerToken,
		AgentToken:      agentToken,
		Status:          "pending",
	}

	if err := saveSchedule(ctx, schedule, cfg.TokenExpiry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	customerURL := fmt.Sprintf("%s/meet/%s", cfg.BaseURL, customerToken)
	agentURL := fmt.Sprintf("%s/meet/%s", cfg.BaseURL, agentToken)

	// Send meeting link to customer via Chatwoot
	if conversationID > 0 && reply != nil {
		msg := fmt.Sprintf("您的视频会议已预约，时间：%s\n点击加入：%s",
			event.Booking.StartTime.Format("2006-01-02 15:04"), customerURL)
		_ = reply(ctx, conversationID, msg)
	}

	// Notify agent via Slack
	if cfg.SlackChannel != "" {
		slackMsg := fmt.Sprintf("新视频会议预约\n客户：%s\n时间：%s\n加入：%s",
			customerName, event.Booking.StartTime.Format("2006-01-02 15:04"), agentURL)
		_ = slack.Send(cfg.SlackChannel, slackMsg)
	}

	c.JSON(http.StatusOK, gin.H{"status": "created", "schedule_id": id})
}

func handleBookingCancelled(ctx context.Context, cfg *Config, event *calcom.Event, reply ReplyFunc, c *gin.Context) {
	schedule, err := findScheduleByBookingID(ctx, event.Booking.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": "not_found"})
		return
	}

	_ = deleteSchedule(ctx, schedule)

	if schedule.ConversationID > 0 && reply != nil {
		_ = reply(ctx, schedule.ConversationID, "视频会议已取消。")
	}

	if cfg.SlackChannel != "" {
		_ = slack.Send(cfg.SlackChannel, fmt.Sprintf("视频会议已取消\n客户：%s", schedule.CustomerName))
	}

	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

func handleBookingRescheduled(ctx context.Context, cfg *Config, event *calcom.Event, reply ReplyFunc, c *gin.Context) {
	schedule, err := findScheduleByBookingID(ctx, event.Booking.ID)
	if err != nil {
		// Treat as new booking if not found
		handleBookingCreated(ctx, cfg, event, reply, c)
		return
	}

	if err := updateScheduleTime(ctx, schedule, event.Booking.StartTime, cfg.TokenExpiry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if schedule.ConversationID > 0 && reply != nil {
		msg := fmt.Sprintf("视频会议时间已更新为：%s", event.Booking.StartTime.Format("2006-01-02 15:04"))
		_ = reply(ctx, schedule.ConversationID, msg)
	}

	if cfg.SlackChannel != "" {
		_ = slack.Send(cfg.SlackChannel, fmt.Sprintf("视频会议已改期\n客户：%s\n新时间：%s",
			schedule.CustomerName, event.Booking.StartTime.Format("2006-01-02 15:04")))
	}

	c.JSON(http.StatusOK, gin.H{"status": "rescheduled"})
}

func handleMeetPage() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Param("token")
		ctx := c.Request.Context()
		cfg := getConfig()

		schedule, err := getScheduleByToken(ctx, token)
		if err != nil {
			renderMeetPage(c, meetData{Error: "链接无效或已过期，请联系客服。"})
			return
		}

		// Determine role based on token
		role := "customer"
		identity := schedule.CustomerEmail
		name := schedule.CustomerName
		if token == schedule.AgentToken {
			role = "agent"
			identity = schedule.AgentEmail
			name = "客服"
		}

		// Create LiveKit room (idempotent)
		if err := createRoom(ctx, schedule.RoomName); err != nil {
			renderMeetPage(c, meetData{Error: "无法创建会议房间，请稍后重试。"})
			return
		}

		// Generate LiveKit participant token
		participantToken, err := createParticipantToken(schedule.RoomName, identity, name)
		if err != nil {
			renderMeetPage(c, meetData{Error: "无法生成会议凭证，请稍后重试。"})
			return
		}

		// Try to activate (dedup Slack notification)
		if role == "customer" {
			if activated, _ := tryActivate(ctx, schedule, cfg.TokenExpiry); activated {
				if cfg.SlackChannel != "" {
					agentURL := fmt.Sprintf("%s/meet/%s", cfg.BaseURL, schedule.AgentToken)
					_ = slack.Send(cfg.SlackChannel, fmt.Sprintf("客户 %s 已进入视频会议，请立即加入：%s",
						schedule.CustomerName, agentURL))
				}
			}
		}

		renderMeetPage(c, meetData{
			ServerURL:   cfg.LiveKit.URL,
			Token:       participantToken,
			ScheduledAt: schedule.ScheduledAt.Format(time.RFC3339),
			Role:        role,
			Status:      schedule.Status,
		})
	}
}

func renderMeetPage(c *gin.Context, data meetData) {
	jsonBytes, _ := json.Marshal(data)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	meetTemplate.Execute(c.Writer, meetPageData{
		DataJSON: template.JS(jsonBytes),
	})
}
