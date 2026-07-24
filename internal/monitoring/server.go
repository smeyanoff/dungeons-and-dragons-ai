package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/llm_log"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/internal/metrics"
	"dungeons-and-dragons-ai/pkg/logger"
)

// Server представляет HTTP сервер для мониторинга LLM запросов
type Server struct {
	httpServer *http.Server
	logRepo    LLMLogRepository
}

// LLMLogRepository интерфейс для работы с логами
type LLMLogRepository interface {
	GetRecent(ctx context.Context, limit int) ([]*llm_log.LLMLog, error)
	GetByID(ctx context.Context, id uint) (*llm_log.LLMLog, error)
	GetByChatID(ctx context.Context, chatID int64, limit int) ([]*llm_log.LLMLog, error)
	GetByTgUserID(ctx context.Context, tgUserID int64, limit int) ([]*llm_log.LLMLog, error)
	GetByDateRange(ctx context.Context, from, to time.Time, limit int) ([]*llm_log.LLMLog, error)
	GetWithErrors(ctx context.Context, limit int) ([]*llm_log.LLMLog, error)
	GetStats(ctx context.Context, from, to time.Time) (*LLMStats, error)
	GetByFilters(ctx context.Context, filters persistence.LLMLogFilters, limit int) ([]*llm_log.LLMLog, error)
	GetBranches(ctx context.Context, filters persistence.LLMLogFilters, limit int) ([]*LLMLogBranch, error)
}

// LLMStats статистика использования LLM (использует тип из persistence)
type LLMStats = persistence.LLMStats

// LLMLogBranch агрегаты по "веткам" запросов (сессиям)
type LLMLogBranch = persistence.LLMLogBranch

// corsMiddleware добавляет CORS заголовки для всех запросов
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Разрешаем запросы из браузера
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Обрабатываем preflight запросы
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// requestTracingMiddleware логирует все входящие запросы для трассировки
func requestTracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Создаем response writer для захвата статуса
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Логируем входящий запрос
		logger.Info("HTTP Request",
			logger.String("method", r.Method),
			logger.String("url", r.URL.Path),
			logger.String("query", r.URL.RawQuery),
			logger.String("user_agent", r.Header.Get("User-Agent")),
			logger.String("remote_addr", r.RemoteAddr),
		)

		// Выполняем запрос
		next.ServeHTTP(wrapped, r)

		// Логируем завершение запроса
		duration := time.Since(start)
		logger.Info("HTTP Response",
			logger.String("method", r.Method),
			logger.String("url", r.URL.Path),
			logger.Int("status", wrapped.statusCode),
			logger.Duration("duration", duration),
		)
	})
}

// responseWriter оборачивает http.ResponseWriter для захвата кода статуса
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// NewServer создает новый сервер мониторинга
func NewServer(addr string, logRepo LLMLogRepository) *Server {
	s := &Server{
		logRepo: logRepo,
	}

	mux := http.NewServeMux()

	// Статические страницы
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/logs", s.handleLogs)
	mux.HandleFunc("/log/", s.handleLogDetail)
	mux.HandleFunc("/errors", s.handleErrors)
	mux.HandleFunc("/branches", s.handleBranches)

	// API endpoints
	mux.HandleFunc("/api/logs", s.handleAPILogs)
	mux.HandleFunc("/api/log/", s.handleAPILogDetail)
	mux.HandleFunc("/api/stats", s.handleAPIStats)
	mux.HandleFunc("/api/branches", s.handleAPIBranches)
	mux.HandleFunc("/api/metrics", s.handleAPIMetrics)

	// Оборачиваем в middleware цепочку: tracing -> cors -> mux
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: requestTracingMiddleware(corsMiddleware(mux)),
	}

	return s
}

// Start запускает HTTP сервер
func (s *Server) Start() error {
	logger.Info("Starting LLM monitoring server",
		logger.String("addr", s.httpServer.Addr),
	)
	return s.httpServer.ListenAndServe()
}

// Shutdown останавливает HTTP сервер
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleIndex обрабатывает главную страницу
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	tmpl := `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>LLM Monitoring</title>
	<style>
		body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
		.container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
		h1 { color: #333; }
		.nav { margin: 20px 0; }
		.nav a { display: inline-block; margin-right: 20px; padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 4px; }
		.nav a:hover { background: #0056b3; }
		.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin: 20px 0; }
		.stat-card { padding: 20px; background: #f8f9fa; border-radius: 4px; border-left: 4px solid #007bff; }
		.stat-value { font-size: 24px; font-weight: bold; color: #007bff; }
		.stat-label { color: #666; margin-top: 5px; }
	</style>
</head>
<body>
	<div class="container">
		<div class="nav">
			<a href="/">Dashboard</a>
			<a href="/logs">Recent Logs</a>
			<a href="/errors">Errors</a>
			<a href="/branches">Branches</a>
		</div>
		<div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
			<h1>🤖 LLM Monitoring Dashboard</h1>
			<button onclick="loadStats()" style="padding: 10px 20px; background: #28a745; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 14px;">🔄 Refresh</button>
		</div>
		<div class="stats" id="stats">
			<div class="stat-card">
				<div class="stat-value" id="total-requests">-</div>
				<div class="stat-label">Total Requests</div>
			</div>
			<div class="stat-card">
				<div class="stat-value" id="total-errors">-</div>
				<div class="stat-label">Total Errors</div>
			</div>
			<div class="stat-card">
				<div class="stat-value" id="total-problems">-</div>
				<div class="stat-label">Total Problems</div>
			</div>
			<div class="stat-card">
				<div class="stat-value" id="avg-duration">-</div>
				<div class="stat-label">Avg Duration (ms)</div>
			</div>
			<div class="stat-card">
				<div class="stat-value" id="total-tokens">-</div>
				<div class="stat-label">Total Tokens</div>
			</div>
			<div class="stat-card">
				<div class="stat-value" id="total-tool-calls">-</div>
				<div class="stat-label">Tool Calls</div>
			</div>
		</div>
	</div>
	<script>
		function loadStats() {
			const from = new Date(Date.now() - 7*24*60*60*1000).toISOString();
			console.log('Loading stats from:', from);

			// Показываем индикатор загрузки
			const statElements = ['total-requests', 'total-errors', 'total-problems', 'avg-duration', 'total-tokens', 'total-tool-calls'];
			statElements.forEach(id => {
				document.getElementById(id).textContent = 'Loading...';
				document.getElementById(id).style.color = '#666';
			});

			fetch('/api/stats?from=' + encodeURIComponent(from))
				.then(r => {
					console.log('API response status:', r.status);
					if (!r.ok) {
						throw new Error('HTTP error! status: ' + r.status);
					}
					return r.json();
				})
				.then(data => {
					console.log('Stats loaded:', data);

					// Обновляем данные с правильными значениями
					document.getElementById('total-requests').textContent = (data.total_requests || 0).toLocaleString();
					document.getElementById('total-errors').textContent = (data.total_errors || 0).toLocaleString();
					document.getElementById('total-problems').textContent = (data.total_problems || data.total_errors || 0).toLocaleString();
					document.getElementById('avg-duration').textContent = data.average_duration_ms ? Math.round(data.average_duration_ms) + 'ms' : '0ms';
					document.getElementById('total-tokens').textContent = (data.total_tokens || 0).toLocaleString();
					document.getElementById('total-tool-calls').textContent = (data.total_tool_calls || 0).toLocaleString();

					// Устанавливаем цвет в зависимости от наличия данных
					statElements.forEach(id => {
						const element = document.getElementById(id);
						element.style.color = (parseInt(element.textContent.replace(/[^\d]/g, '')) > 0) ? '#007bff' : '#666';
					});
				})
				.catch(err => {
					console.error('Failed to load stats:', err);
					statElements.forEach(id => {
						document.getElementById(id).textContent = 'Error loading data';
						document.getElementById(id).style.color = '#dc3545';
					});
				});
		}

		// Загружаем статистику при загрузке страницы
		loadStats();

		// Обновляем каждые 30 секунд
		setInterval(loadStats, 30000);
	</script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, tmpl)
}

// handleLogs обрабатывает страницу со списком логов
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	chatIDStr := r.URL.Query().Get("chat_id")
	tgUserIDStr := r.URL.Query().Get("tg_user_id")
	sessionIDStr := r.URL.Query().Get("session_id")
	var logs []*llm_log.LLMLog
	var err error
	filters := persistence.LLMLogFilters{}
	hasFilters := false
	if chatIDStr != "" {
		if chatID, parseErr := strconv.ParseInt(chatIDStr, 10, 64); parseErr == nil {
			filters.ChatID = &chatID
			hasFilters = true
		}
	}
	if tgUserIDStr != "" {
		if tgUserID, parseErr := strconv.ParseInt(tgUserIDStr, 10, 64); parseErr == nil {
			filters.TgUserID = &tgUserID
			hasFilters = true
		}
	}
	if sessionIDStr != "" {
		if sessionID, parseErr := strconv.ParseUint(sessionIDStr, 10, 32); parseErr == nil {
			sessionIDUint := uint(sessionID)
			filters.SessionID = &sessionIDUint
			hasFilters = true
		}
	}

	if hasFilters {
		logs, err = s.logRepo.GetByFilters(ctx, filters, limit)
	} else {
		logs, err = s.logRepo.GetRecent(ctx, limit)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get logs: %v", err), http.StatusInternalServerError)
		return
	}

	renderLogsPage(w, logs, logsPageFilters{
		ChatID:    chatIDStr,
		TgUserID:  tgUserIDStr,
		SessionID: sessionIDStr,
		Limit:     limit,
	})
}

// handleLogDetail обрабатывает страницу деталей лога
func (s *Server) handleLogDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := r.URL.Path[len("/log/"):]
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid log ID", http.StatusBadRequest)
		return
	}

	logEntry, err := s.logRepo.GetByID(ctx, uint(id))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get log: %v", err), http.StatusInternalServerError)
		return
	}

	if logEntry == nil {
		http.NotFound(w, r)
		return
	}

	renderLogDetailPage(w, logEntry)
}

// handleErrors обрабатывает страницу с ошибками
func (s *Server) handleErrors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	logs, err := s.logRepo.GetWithErrors(ctx, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get error logs: %v", err), http.StatusInternalServerError)
		return
	}

	renderLogsPage(w, logs, logsPageFilters{Limit: limit})
}

// handleStats обрабатывает страницу статистики
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	from := time.Now().Add(-7 * 24 * time.Hour)
	to := time.Now()

	if fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		} else {
			logger.Warn("Failed to parse 'from' parameter in handleStats",
				logger.String("from", fromStr),
				logger.ErrorField(err),
			)
		}
	}
	if toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		} else {
			logger.Warn("Failed to parse 'to' parameter in handleStats",
				logger.String("to", toStr),
				logger.ErrorField(err),
			)
		}
	}

	stats, err := s.logRepo.GetStats(ctx, from, to)
	if err != nil {
		logger.Error("Failed to get LLM stats",
			logger.ErrorField(err),
			logger.Time("from", from),
			logger.Time("to", to),
		)
		http.Error(w, fmt.Sprintf("Failed to get stats: %v", err), http.StatusInternalServerError)
		return
	}

	renderStatsPage(w, stats, from, to)
}

// handleBranches обрабатывает страницу веток запросов (сессий)
func (s *Server) handleBranches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	chatIDStr := r.URL.Query().Get("chat_id")
	tgUserIDStr := r.URL.Query().Get("tg_user_id")
	filters := persistence.LLMLogFilters{}
	hasFilters := false
	if chatIDStr != "" {
		if chatID, parseErr := strconv.ParseInt(chatIDStr, 10, 64); parseErr == nil {
			filters.ChatID = &chatID
			hasFilters = true
		}
	}
	if tgUserIDStr != "" {
		if tgUserID, parseErr := strconv.ParseInt(tgUserIDStr, 10, 64); parseErr == nil {
			filters.TgUserID = &tgUserID
			hasFilters = true
		}
	}

	var branches []*LLMLogBranch
	var err error
	if hasFilters {
		branches, err = s.logRepo.GetBranches(ctx, filters, limit)
	} else {
		branches, err = s.logRepo.GetBranches(ctx, persistence.LLMLogFilters{}, limit)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get branches: %v", err), http.StatusInternalServerError)
		return
	}

	renderBranchesPage(w, branches, branchPageFilters{
		ChatID:   chatIDStr,
		TgUserID: tgUserIDStr,
		Limit:    limit,
	})
}

// handleAPILogs обрабатывает API запрос для получения логов
func (s *Server) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	chatIDStr := r.URL.Query().Get("chat_id")
	tgUserIDStr := r.URL.Query().Get("tg_user_id")
	sessionIDStr := r.URL.Query().Get("session_id")
	var logs []*llm_log.LLMLog
	var err error
	filters := persistence.LLMLogFilters{}
	hasFilters := false
	if chatIDStr != "" {
		if chatID, parseErr := strconv.ParseInt(chatIDStr, 10, 64); parseErr == nil {
			filters.ChatID = &chatID
			hasFilters = true
		}
	}
	if tgUserIDStr != "" {
		if tgUserID, parseErr := strconv.ParseInt(tgUserIDStr, 10, 64); parseErr == nil {
			filters.TgUserID = &tgUserID
			hasFilters = true
		}
	}
	if sessionIDStr != "" {
		if sessionID, parseErr := strconv.ParseUint(sessionIDStr, 10, 32); parseErr == nil {
			sessionIDUint := uint(sessionID)
			filters.SessionID = &sessionIDUint
			hasFilters = true
		}
	}
	if hasFilters {
		logs, err = s.logRepo.GetByFilters(ctx, filters, limit)
	} else {
		logs, err = s.logRepo.GetRecent(ctx, limit)
	}

	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get logs: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, logs)
}

// handleAPIBranches обрабатывает API запрос для получения веток
func (s *Server) handleAPIBranches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	chatIDStr := r.URL.Query().Get("chat_id")
	tgUserIDStr := r.URL.Query().Get("tg_user_id")
	filters := persistence.LLMLogFilters{}
	hasFilters := false
	if chatIDStr != "" {
		if chatID, parseErr := strconv.ParseInt(chatIDStr, 10, 64); parseErr == nil {
			filters.ChatID = &chatID
			hasFilters = true
		}
	}
	if tgUserIDStr != "" {
		if tgUserID, parseErr := strconv.ParseInt(tgUserIDStr, 10, 64); parseErr == nil {
			filters.TgUserID = &tgUserID
			hasFilters = true
		}
	}

	var branches []*LLMLogBranch
	var err error
	if hasFilters {
		branches, err = s.logRepo.GetBranches(ctx, filters, limit)
	} else {
		branches, err = s.logRepo.GetBranches(ctx, persistence.LLMLogFilters{}, limit)
	}
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get branches: %v", err))
		return
	}

	// Ensure we return an empty slice instead of nil for proper JSON serialization
	if branches == nil {
		branches = make([]*LLMLogBranch, 0)
	}

	respondJSON(w, http.StatusOK, branches)
}

// handleAPILogDetail обрабатывает API запрос для получения деталей лога
func (s *Server) handleAPILogDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := r.URL.Path[len("/api/log/"):]
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		respondJSONError(w, http.StatusBadRequest, "Invalid log ID")
		return
	}

	logEntry, err := s.logRepo.GetByID(ctx, uint(id))
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get log: %v", err))
		return
	}

	if logEntry == nil {
		respondJSONError(w, http.StatusNotFound, "Log not found")
		return
	}

	respondJSON(w, http.StatusOK, logEntry)
}

// handleAPIStats обрабатывает API запрос для получения статистики
func (s *Server) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	from := time.Now().Add(-7 * 24 * time.Hour)
	to := time.Now()

	if fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		} else {
			logger.Warn("Failed to parse 'from' parameter in handleAPIStats",
				logger.String("from", fromStr),
				logger.ErrorField(err),
			)
		}
	}
	if toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		} else {
			logger.Warn("Failed to parse 'to' parameter in handleAPIStats",
				logger.String("to", toStr),
				logger.ErrorField(err),
			)
		}
	}

	stats, err := s.logRepo.GetStats(ctx, from, to)
	if err != nil {
		logger.Error("Failed to get LLM stats",
			logger.ErrorField(err),
			logger.Time("from", from),
			logger.Time("to", to),
		)
		respondJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get stats: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, stats)
}

// handleAPIMetrics возвращает счётчики мониторинга: RAG-failures, утечки системного текста (output-guard).
// GigaChat 402 доступен через GetMetrics() клиента GigaChat при наличии доступа к нему.
func (s *Server) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	out := map[string]int64{
		"rag_failure_count":            metrics.RAGFailureCount(),
		"output_leak_count":            metrics.OutputLeakCount(),
		"rag_empty_result_count":       metrics.RAGEmptyResultCount(),
		"telegram_polling_error_count": metrics.TelegramPollingErrorCount(),
	}
	respondJSON(w, http.StatusOK, out)
}

// Helper functions

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondJSONError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func formatInt64(v int64) string {
	if v == 0 {
		return "-"
	}
	return strconv.FormatInt(v, 10)
}

func formatUintPtr(v *uint) string {
	if v == nil || *v == 0 {
		return "-"
	}
	return strconv.FormatUint(uint64(*v), 10)
}

func toolNames(raw interface{}) string {
	var payload string
	switch v := raw.(type) {
	case string:
		payload = v
	case *string:
		if v != nil {
			payload = *v
		}
	}
	if strings.TrimSpace(payload) == "" {
		return "-"
	}
	var calls []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(payload), &calls); err != nil {
		return "-"
	}
	if len(calls) == 0 {
		return "-"
	}
	seen := make(map[string]bool)
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, ", ")
}

type logsPageFilters struct {
	ChatID    string
	TgUserID  string
	SessionID string
	Limit     int
}

type branchPageFilters struct {
	ChatID   string
	TgUserID string
	Limit    int
}

func renderLogsPage(w http.ResponseWriter, logs []*llm_log.LLMLog, filters logsPageFilters) {
	tmpl := template.Must(template.New("logs").Funcs(template.FuncMap{
		"truncate": func(s string, maxLen int) string {
			if len(s) <= maxLen {
				return s
			}
			return s[:maxLen] + "..."
		},
		"escape": func(s string) template.HTML {
			return template.HTML(template.HTMLEscapeString(s))
		},
		"formatInt64":   formatInt64,
		"formatUintPtr": formatUintPtr,
		"toolNames":     toolNames,
	}).Parse(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>LLM Logs</title>
	<style>
		body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
		.container { max-width: 1800px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; }
		table { width: 100%; border-collapse: collapse; margin-top: 20px; }
		th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; vertical-align: top; }
		th { background: #007bff; color: white; position: sticky; top: 0; }
		tr:hover { background: #f5f5f5; }
		.prompt, .response { 
			max-width: 500px; 
			max-height: 150px; 
			overflow-y: auto;
			word-wrap: break-word;
			white-space: pre-wrap;
			font-size: 0.9em;
			line-height: 1.4;
			cursor: pointer;
			position: relative;
		}
		.prompt.expanded, .response.expanded {
			max-height: none;
			position: relative;
			z-index: 10;
			background: #fff;
			box-shadow: 0 2px 8px rgba(0,0,0,0.1);
			border: 1px solid #ddd;
			border-radius: 4px;
			padding: 8px;
		}
		.expand-btn {
			color: #007bff;
			cursor: pointer;
			font-size: 0.8em;
			margin-left: 5px;
			text-decoration: underline;
		}
		.expand-btn:hover {
			color: #0056b3;
		}
		.error { color: red; }
		.time { font-size: 0.9em; color: #666; }
		.badge { padding: 2px 8px; border-radius: 12px; font-size: 0.8em; }
		.badge-success { background: #d4edda; color: #155724; }
		.badge-error { background: #f8d7da; color: #721c24; }
		.badge-tools { background: #d1ecf1; color: #0c5460; }
		a { color: #007bff; text-decoration: none; }
		a:hover { text-decoration: underline; }
		.nav { margin: 20px 0; }
		.nav a { display: inline-block; margin-right: 20px; padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 4px; }
		.nav a:hover { background: #0056b3; }
		.modal {
			display: none;
			position: fixed;
			z-index: 1000;
			left: 0;
			top: 0;
			width: 100%;
			height: 100%;
			overflow: auto;
			background-color: rgba(0,0,0,0.5);
		}
		.modal-content {
			background-color: #fefefe;
			margin: 5% auto;
			padding: 20px;
			border: 1px solid #888;
			border-radius: 8px;
			width: 80%;
			max-width: 1000px;
			max-height: 80vh;
			overflow-y: auto;
		}
		.close {
			color: #aaa;
			float: right;
			font-size: 28px;
			font-weight: bold;
			cursor: pointer;
		}
		.close:hover { color: #000; }
		.modal-text {
			white-space: pre-wrap;
			word-wrap: break-word;
			font-family: monospace;
			font-size: 0.9em;
			background: #f8f9fa;
			padding: 15px;
			border-radius: 4px;
			max-height: 60vh;
			overflow-y: auto;
		}
	</style>
</head>
<body>
	<div class="container">
		<h1>LLM Logs</h1>
		<div class="nav">
			<a href="/">Dashboard</a>
			<a href="/logs">Recent Logs</a>
			<a href="/errors">Errors</a>
			<a href="/branches">Branches</a>
		</div>
		<form method="get" action="/logs" style="margin: 20px 0; display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 10px;">
			<input type="text" name="chat_id" placeholder="Chat ID" value="{{.Filters.ChatID}}">
			<input type="text" name="tg_user_id" placeholder="TG User ID" value="{{.Filters.TgUserID}}">
			<input type="text" name="session_id" placeholder="Session ID" value="{{.Filters.SessionID}}">
			<input type="number" name="limit" placeholder="Limit" min="1" max="1000" value="{{.Filters.Limit}}">
			<button type="submit" style="padding: 8px 12px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer;">Filter</button>
		</form>
		<table>
			<thead>
				<tr>
					<th style="width: 60px;">ID</th>
					<th style="width: 150px;">Time</th>
					<th style="width: 120px;">Chat ID</th>
					<th style="width: 120px;">User ID</th>
					<th style="width: 120px;">Session ID</th>
					<th style="width: 120px;">Branch</th>
					<th style="width: 100px;">Model</th>
					<th style="width: 500px;">Prompt</th>
					<th style="width: 500px;">Response</th>
					<th style="width: 100px;">Duration (ms)</th>
					<th style="width: 80px;">Tokens</th>
					<th style="width: 80px;">Status</th>
					<th style="width: 80px;">Tools</th>
				</tr>
			</thead>
			<tbody>
				{{range .Logs}}
				<tr>
					<td><a href="/log/{{.ID}}">{{.ID}}</a></td>
					<td class="time">{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
					<td>{{formatInt64 .ChatID}}</td>
					<td>{{formatInt64 .TgUserID}}</td>
					<td>{{formatUintPtr .SessionID}}</td>
					<td>
						{{if .SessionID}}
						<a href="/logs?chat_id={{.ChatID}}&session_id={{.SessionID}}">Branch</a>
						{{else}}
						<a href="/logs?chat_id={{.ChatID}}">Chat</a>
						{{end}}
					</td>
					<td>{{.Model}}</td>
					<td class="prompt" id="prompt-{{.ID}}" onclick="toggleExpand('prompt-{{.ID}}', 'prompt-modal-{{.ID}}')">
						{{truncate .Prompt 500}}
						{{if gt (len .Prompt) 500}}
						<span class="expand-btn">[Развернуть]</span>
						{{end}}
						<div id="prompt-modal-{{.ID}}" class="modal" onclick="event.stopPropagation(); this.style.display='none';">
							<div class="modal-content" onclick="event.stopPropagation();">
								<span class="close" onclick="document.getElementById('prompt-modal-{{.ID}}').style.display='none'">&times;</span>
								<h3>Prompt (Log #{{.ID}})</h3>
								<div class="modal-text">{{escape .Prompt}}</div>
							</div>
						</div>
					</td>
					<td class="response" id="response-{{.ID}}" onclick="toggleExpand('response-{{.ID}}', 'response-modal-{{.ID}}')">
						{{truncate .Response 500}}
						{{if gt (len .Response) 500}}
						<span class="expand-btn">[Развернуть]</span>
						{{end}}
						<div id="response-modal-{{.ID}}" class="modal" onclick="event.stopPropagation(); this.style.display='none';">
							<div class="modal-content" onclick="event.stopPropagation();">
								<span class="close" onclick="document.getElementById('response-modal-{{.ID}}').style.display='none'">&times;</span>
								<h3>Response (Log #{{.ID}})</h3>
								<div class="modal-text">{{escape .Response}}</div>
							</div>
						</div>
					</td>
					<td>{{.DurationMs}}</td>
					<td>{{if .TokensUsed}}{{.TokensUsed}}{{else}}-{{end}}</td>
					<td>
						{{if .Error}}
						<span class="badge badge-error">Error</span>
						{{else}}
						<span class="badge badge-success">OK</span>
						{{end}}
					</td>
					<td>{{toolNames .ToolsCalls}}</td>
				</tr>
				{{end}}
			</tbody>
		</table>
	</div>
	<script>
		function toggleExpand(cellId, modalId) {
			const cell = document.getElementById(cellId);
			const modal = document.getElementById(modalId);
			if (cell.classList.contains('expanded')) {
				cell.classList.remove('expanded');
			} else {
				cell.classList.add('expanded');
			}
			// Также открываем модальное окно при двойном клике
			if (event.detail === 2) {
				modal.style.display = 'block';
			}
		}
		// Закрытие модального окна при клике вне его
		window.onclick = function(event) {
			if (event.target.classList.contains('modal')) {
				event.target.style.display = 'none';
			}
		}
	</script>
</body>
</html>
	`))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	tmpl.Execute(w, map[string]interface{}{
		"Logs":    logs,
		"Filters": filters,
	})
}

func renderLogDetailPage(w http.ResponseWriter, logEntry *llm_log.LLMLog) {
	tmpl := template.Must(template.New("log-detail").Funcs(template.FuncMap{
		"escape": func(s string) template.HTML {
			return template.HTML(template.HTMLEscapeString(s))
		},
		"formatInt64":   formatInt64,
		"formatUintPtr": formatUintPtr,
		"toolNames":     toolNames,
	}).Parse(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>Log #{{.ID}}</title>
	<style>
		body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
		.container { max-width: 1400px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; }
		.field { margin: 20px 0; }
		.label { font-weight: bold; color: #666; margin-bottom: 8px; }
		.value { margin-top: 5px; padding: 15px; background: #f8f9fa; border-radius: 4px; white-space: pre-wrap; word-wrap: break-word; font-family: 'Courier New', monospace; font-size: 0.95em; line-height: 1.6; max-height: 600px; overflow-y: auto; border: 1px solid #dee2e6; }
		.value-long { max-height: none; }
		.error { background: #f8d7da; color: #721c24; border-color: #f5c6cb; }
		.json { font-family: 'Courier New', monospace; font-size: 0.9em; }
		.nav { margin: 20px 0; }
		.nav a { display: inline-block; margin-right: 20px; padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 4px; }
		.nav a:hover { background: #0056b3; }
		.meta-info { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin-bottom: 30px; }
		.meta-field { padding: 10px; background: #e9ecef; border-radius: 4px; }
		.meta-label { font-weight: bold; color: #495057; font-size: 0.9em; }
		.meta-value { margin-top: 5px; color: #212529; }
		.expand-btn { color: #007bff; cursor: pointer; font-size: 0.85em; margin-top: 10px; text-decoration: underline; }
		.expand-btn:hover { color: #0056b3; }
	</style>
</head>
<body>
	<div class="container">
		<h1>Log #{{.ID}}</h1>
		<div class="nav">
			<a href="/">Dashboard</a>
			<a href="/logs">Recent Logs</a>
			<a href="/errors">Errors</a>
			<a href="/branches">Branches</a>
		</div>
		<div class="meta-info">
			<div class="meta-field">
				<div class="meta-label">Time</div>
				<div class="meta-value">{{.CreatedAt.Format "2006-01-02 15:04:05 MST"}}</div>
			</div>
			<div class="meta-field">
				<div class="meta-label">Chat ID</div>
				<div class="meta-value">{{formatInt64 .ChatID}}</div>
			</div>
			<div class="meta-field">
				<div class="meta-label">User ID</div>
				<div class="meta-value">{{formatInt64 .TgUserID}}</div>
			</div>
			<div class="meta-field">
				<div class="meta-label">Session ID</div>
				<div class="meta-value">{{formatUintPtr .SessionID}}</div>
			</div>
			<div class="meta-field">
				<div class="meta-label">Request Branch</div>
				<div class="meta-value">
					{{if .SessionID}}
					<a href="/logs?chat_id={{.ChatID}}&session_id={{.SessionID}}">View branch</a>
					{{else}}
					<a href="/logs?chat_id={{.ChatID}}">View chat logs</a>
					{{end}}
				</div>
			</div>
			<div class="meta-field">
				<div class="meta-label">Model</div>
				<div class="meta-value">{{.Model}}</div>
			</div>
			<div class="meta-field">
				<div class="meta-label">Duration (ms)</div>
				<div class="meta-value">{{.DurationMs}}</div>
			</div>
			{{if .TokensUsed}}
			<div class="meta-field">
				<div class="meta-label">Tokens Used</div>
				<div class="meta-value">{{.TokensUsed}}</div>
			</div>
			{{end}}
			{{if .MaxTokens}}
			<div class="meta-field">
				<div class="meta-label">Max Tokens</div>
				<div class="meta-value">{{.MaxTokens}}</div>
			</div>
			{{end}}
		</div>
		<div class="field">
			<div class="label">Prompt</div>
			<div class="value" id="prompt-value">{{escape .Prompt}}</div>
			{{if gt (len .Prompt) 2000}}
			<span class="expand-btn" onclick="toggleFullHeight('prompt-value')">[Показать полностью]</span>
			{{end}}
		</div>
		<div class="field">
			<div class="label">Response</div>
			<div class="value" id="response-value">{{escape .Response}}</div>
			{{if gt (len .Response) 2000}}
			<span class="expand-btn" onclick="toggleFullHeight('response-value')">[Показать полностью]</span>
			{{end}}
		</div>
		{{if .Error}}
		<div class="field">
			<div class="label">Error</div>
			<div class="value error">{{escape .Error}}</div>
		</div>
		{{end}}
			<div class="field">
				<div class="label">Tool Names</div>
				<div class="value json">{{toolNames .ToolsCalls}}</div>
			</div>
			{{if .ToolsCalls}}
			<div class="field">
				<div class="label">Tools Calls</div>
				<div class="value json" id="tools-value">{{escape .ToolsCalls}}</div>
				{{if gt (len .ToolsCalls) 2000}}
				<span class="expand-btn" onclick="toggleFullHeight('tools-value')">[Показать полностью]</span>
				{{end}}
			</div>
			{{end}}
	</div>
	<script>
		function toggleFullHeight(elementId) {
			const element = document.getElementById(elementId);
			if (element.classList.contains('value-long')) {
				element.classList.remove('value-long');
				element.previousElementSibling.previousElementSibling.textContent = '[Показать полностью]';
			} else {
				element.classList.add('value-long');
				element.previousElementSibling.previousElementSibling.textContent = '[Свернуть]';
			}
		}
	</script>
</body>
</html>
	`))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	tmpl.Execute(w, logEntry)
}

func renderBranchesPage(w http.ResponseWriter, branches []*LLMLogBranch, filters branchPageFilters) {
	tmpl := template.Must(template.New("branches").Parse(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>LLM Branches</title>
	<style>
		body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
		.container { max-width: 1400px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; }
		table { width: 100%; border-collapse: collapse; margin-top: 20px; }
		th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; vertical-align: top; }
		th { background: #007bff; color: white; position: sticky; top: 0; }
		tr:hover { background: #f5f5f5; }
		.nav { margin: 20px 0; }
		.nav a { display: inline-block; margin-right: 20px; padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 4px; }
		.nav a:hover { background: #0056b3; }
		.time { font-size: 0.9em; color: #666; }
	</style>
</head>
<body>
	<div class="container">
		<h1>LLM Branches</h1>
		<div class="nav">
			<a href="/">Dashboard</a>
			<a href="/logs">Recent Logs</a>
			<a href="/errors">Errors</a>
			<a href="/branches">Branches</a>
		</div>
		<form method="get" action="/branches" style="margin: 20px 0; display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 10px;">
			<input type="text" name="chat_id" placeholder="Chat ID" value="{{.Filters.ChatID}}">
			<input type="text" name="tg_user_id" placeholder="TG User ID" value="{{.Filters.TgUserID}}">
			<input type="number" name="limit" placeholder="Limit" min="1" max="1000" value="{{.Filters.Limit}}">
			<button type="submit" style="padding: 8px 12px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer;">Filter</button>
		</form>
		<table>
			<thead>
				<tr>
					<th style="width: 120px;">Branch ID</th>
					<th style="width: 120px;">Chat ID</th>
					<th style="width: 120px;">User ID</th>
					<th style="width: 140px;">First Seen</th>
					<th style="width: 140px;">Last Seen</th>
					<th style="width: 120px;">Requests</th>
					<th style="width: 120px;">Errors</th>
					<th style="width: 120px;">Tokens</th>
					<th style="width: 120px;">Tool Calls</th>
					<th style="width: 120px;">Open Logs</th>
				</tr>
			</thead>
			<tbody>
				{{range .Branches}}
				<tr>
					<td>{{.SessionID}}</td>
					<td>{{.ChatID}}</td>
					<td>{{.TgUserID}}</td>
					<td class="time">{{.FirstSeen.Format "2006-01-02 15:04:05"}}</td>
					<td class="time">{{.LastSeen.Format "2006-01-02 15:04:05"}}</td>
					<td>{{.TotalRequests}}</td>
					<td>{{.TotalErrors}}</td>
					<td>{{.TotalTokens}}</td>
					<td>{{.TotalToolCalls}}</td>
					<td><a href="/logs?chat_id={{.ChatID}}&session_id={{.SessionID}}">View</a></td>
				</tr>
				{{end}}
			</tbody>
		</table>
	</div>
</body>
</html>
	`))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	tmpl.Execute(w, map[string]interface{}{
		"Branches": branches,
		"Filters":  filters,
	})
}

func renderStatsPage(w http.ResponseWriter, stats *LLMStats, from, to time.Time) {
	// Защита от nil stats
	if stats == nil {
		stats = &LLMStats{
			TotalRequests:     0,
			TotalErrors:       0,
			AverageDurationMs: 0,
			TotalTokens:       0,
			TotalToolCalls:    0,
			TotalProblems:     0,
		}
	}

	tmpl := template.Must(template.New("stats").Parse(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>Statistics</title>
	<style>
		body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
		.container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
		h1 { color: #333; }
		.nav { margin: 20px 0; }
		.nav a { display: inline-block; margin-right: 20px; padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 4px; }
		.nav a:hover { background: #0056b3; }
		.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin: 20px 0; }
		.stat-card { padding: 20px; background: #f8f9fa; border-radius: 4px; border-left: 4px solid #007bff; }
		.stat-value { font-size: 24px; font-weight: bold; color: #007bff; }
		.stat-label { color: #666; margin-top: 5px; }
		.period { color: #666; font-size: 0.9em; margin-bottom: 20px; }
	</style>
</head>
<body>
	<div class="container">
		<h1>📊 Statistics</h1>
		<div class="nav">
			<a href="/">Dashboard</a>
			<a href="/logs">Recent Logs</a>
			<a href="/errors">Errors</a>
			<a href="/branches">Branches</a>
		</div>
		<div class="period">
			<strong>Period:</strong> {{.From.Format "2006-01-02 15:04:05"}} to {{.To.Format "2006-01-02 15:04:05"}}
		</div>
		<div class="stats">
			<div class="stat-card">
				<div class="stat-value">{{.Stats.TotalRequests}}</div>
				<div class="stat-label">Total Requests</div>
			</div>
			<div class="stat-card">
				<div class="stat-value">{{.Stats.TotalErrors}}</div>
				<div class="stat-label">Total Errors</div>
			</div>
			<div class="stat-card">
				<div class="stat-value">{{.Stats.TotalProblems}}</div>
				<div class="stat-label">Total Problems</div>
			</div>
			<div class="stat-card">
				<div class="stat-value">{{.Stats.AverageDurationMs}}</div>
				<div class="stat-label">Avg Duration (ms)</div>
			</div>
			<div class="stat-card">
				<div class="stat-value">{{.Stats.TotalTokens}}</div>
				<div class="stat-label">Total Tokens</div>
			</div>
			<div class="stat-card">
				<div class="stat-value">{{.Stats.TotalToolCalls}}</div>
				<div class="stat-label">Tool Calls</div>
			</div>
		</div>
	</div>
</body>
</html>
	`))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	tmpl.Execute(w, map[string]interface{}{
		"Stats": stats,
		"From":  from,
		"To":    to,
	})
}
