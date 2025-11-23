package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/yachiko/remora/internal/database"
	"github.com/yachiko/remora/internal/models"
	"go.uber.org/zap"
)

type Handler struct {
	repo   database.ReminderRepository
	logger *zap.Logger
}

func NewHandler(repo database.ReminderRepository, logger *zap.Logger) *Handler {
	return &Handler{repo: repo, logger: logger}
}

func (h *Handler) ListReminders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	limit := 50
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	repository := query.Get("repository")
	issue := query.Get("issue")
	status := query.Get("status")
	user := query.Get("user")

	db := database.DB
	dbQuery := db.Model(&models.Reminder{})

	if repository != "" {
		dbQuery = dbQuery.Where("repository_owner || '/' || repository_name = ?", repository)
	}
	if issue != "" {
		if issueNum, err := strconv.Atoi(issue); err == nil {
			dbQuery = dbQuery.Where("issue_number = ?", issueNum)
		}
	}
	if status != "" {
		dbQuery = dbQuery.Where("status = ?", status)
	}
	if user != "" {
		dbQuery = dbQuery.Where("requester_username = ?", user)
	}

	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		h.logger.Error("failed to count reminders", zap.Error(err))
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var reminders []*models.Reminder
	err := dbQuery.Order("created_at DESC").Limit(limit).Offset(offset).Find(&reminders).Error
	if err != nil {
		h.logger.Error("failed to query reminders", zap.Error(err))
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"data": reminders,
		"meta": map[string]interface{}{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
