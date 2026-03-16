package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api   *tgbotapi.BotAPI
	cfg   *Config
	db    *DB
	proxy *ProxyManager
}

func NewBot(cfg *Config, db *DB, proxy *ProxyManager) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}

	slog.Info("bot authorized", "username", api.Self.UserName)

	// Register command menu so users see buttons.
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Запросить доступ к прокси"},
		tgbotapi.BotCommand{Command: "my", Description: "Мои прокси-ссылки"},
		tgbotapi.BotCommand{Command: "more", Description: "Доп. сессия для устройства"},
	)
	if _, err := api.Request(commands); err != nil {
		slog.Warn("set commands", "err", err)
	}

	return &Bot{api: api, cfg: cfg, db: db, proxy: proxy}, nil
}

func (b *Bot) Run() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)
	for update := range updates {
		if update.Message != nil {
			b.handleMessage(update.Message)
		}
		if update.CallbackQuery != nil {
			b.handleCallback(update.CallbackQuery)
		}
	}
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	if !msg.IsCommand() {
		return
	}

	// Admin commands.
	if msg.From.ID == b.cfg.AdminID {
		switch msg.Command() {
		case "users":
			b.cmdUsers(msg)
			return
		case "stats":
			b.cmdStats(msg)
			return
		case "revoke":
			b.cmdRevoke(msg)
			return
		case "kick":
			b.cmdKick(msg)
			return
		}
	}

	// User commands.
	switch msg.Command() {
	case "start":
		b.cmdStart(msg)
	case "more":
		b.cmdMore(msg)
	case "my":
		b.cmdMy(msg)
	default:
		b.send(msg.Chat.ID, "Неизвестная команда. Доступные: /start, /more, /my")
	}
}

func (b *Bot) cmdStart(msg *tgbotapi.Message) {
	user, err := b.db.GetUserByTelegramID(msg.From.ID)
	if err == nil {
		switch user.Status {
		case "approved":
			b.send(msg.Chat.ID, "У тебя уже есть доступ. Используй /my чтобы увидеть свои прокси.")
		case "pending":
			b.send(msg.Chat.ID, "Твоя заявка на рассмотрении. Подожди немного.")
		case "banned":
			b.send(msg.Chat.ID, "Доступ заблокирован.")
		}
		return
	}

	username := msg.From.UserName
	if username == "" {
		username = msg.From.FirstName
	}

	_, err = b.db.CreateUser(msg.From.ID, username)
	if err != nil {
		slog.Error("create user", "err", err)
		b.send(msg.Chat.ID, "Произошла ошибка, попробуй позже.")
		return
	}

	b.send(msg.Chat.ID, "Заявка отправлена! Подожди пока администратор её рассмотрит.")

	// Notify admin.
	text := fmt.Sprintf("Новая заявка на доступ:\n\nПользователь: @%s\nID: %d", username, msg.From.ID)
	adminMsg := tgbotapi.NewMessage(b.cfg.AdminID, text)
	adminMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Одобрить", fmt.Sprintf("a:%d", msg.From.ID)),
			tgbotapi.NewInlineKeyboardButtonData("Отклонить", fmt.Sprintf("d:%d", msg.From.ID)),
		),
	)
	b.api.Send(adminMsg)
}

func (b *Bot) cmdMore(msg *tgbotapi.Message) {
	user, err := b.db.GetUserByTelegramID(msg.From.ID)
	if err != nil || user.Status != "approved" {
		b.send(msg.Chat.ID, "У тебя нет доступа. Отправь /start чтобы запросить.")
		return
	}

	deviceName := strings.TrimSpace(msg.CommandArguments())
	if deviceName == "" {
		b.send(msg.Chat.ID, "Укажи имя устройства: /more Ноутбук")
		return
	}

	// Pre-create inactive secret.
	hexSecret, err := b.proxy.GenerateSecret()
	if err != nil {
		slog.Error("generate secret", "err", err)
		b.send(msg.Chat.ID, "Ошибка генерации, попробуй позже.")
		return
	}

	_, err = b.db.CreateSecret(user.ID, hexSecret, deviceName, false)
	if err != nil {
		slog.Error("create secret", "err", err)
		b.send(msg.Chat.ID, "Ошибка, попробуй позже.")
		return
	}

	count, _ := b.db.CountActiveSecrets(msg.From.ID)

	b.send(msg.Chat.ID, fmt.Sprintf("Запрос на доп. сессию (%s) отправлен.", deviceName))

	// Notify admin.
	username := user.Username
	text := fmt.Sprintf("Запрос доп. сессии:\n\nПользователь: @%s\nУстройство: %s\nАктивных сессий: %d", username, deviceName, count)
	adminMsg := tgbotapi.NewMessage(b.cfg.AdminID, text)
	adminMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Одобрить", fmt.Sprintf("sa:%d", msg.From.ID)),
			tgbotapi.NewInlineKeyboardButtonData("Отклонить", fmt.Sprintf("sd:%d", msg.From.ID)),
		),
	)
	b.api.Send(adminMsg)
}

func (b *Bot) cmdMy(msg *tgbotapi.Message) {
	secrets, err := b.db.GetSecretsByTelegramID(msg.From.ID)
	if err != nil || len(secrets) == 0 {
		b.send(msg.Chat.ID, "У тебя нет активных прокси.")
		return
	}

	var lines []string
	for i, s := range secrets {
		name := s.DeviceName
		if name == "" {
			name = "Основной"
		}
		link := b.proxy.ProxyLink(s.HexSecret)
		lines = append(lines, fmt.Sprintf("%d. %s\n%s", i+1, name, link))
	}

	b.send(msg.Chat.ID, "Твои прокси:\n\n"+strings.Join(lines, "\n\n"))
}

func (b *Bot) cmdUsers(msg *tgbotapi.Message) {
	users, counts, err := b.db.ListApprovedUsers()
	if err != nil {
		slog.Error("list users", "err", err)
		b.send(msg.Chat.ID, "Ошибка получения списка.")
		return
	}

	if len(users) == 0 {
		b.send(msg.Chat.ID, "Пользователей нет.")
		return
	}

	var lines []string
	for _, u := range users {
		status := u.Status
		if status == "approved" {
			status = "✓"
		} else {
			status = "⏳"
		}
		lines = append(lines, fmt.Sprintf("%s @%s — %d сессий", status, u.Username, counts[u.TelegramID]))
	}

	b.send(msg.Chat.ID, "Пользователи:\n\n"+strings.Join(lines, "\n"))
}

func (b *Bot) cmdStats(msg *tgbotapi.Message) {
	stats, err := FetchProxyStats(b.cfg.MetricsURL)
	if err != nil {
		slog.Error("fetch stats", "err", err)
		b.send(msg.Chat.ID, "Не удалось получить статистику. Прокси не запущен?")
		return
	}

	if len(stats) == 0 {
		b.send(msg.Chat.ID, "Нет активных пользователей.")
		return
	}

	labelMap, _ := b.db.SecretLabelToUser()

	var lines []string
	for _, s := range stats {
		name := labelMap[s.Label]
		if name == "" {
			name = s.Label
		}
		status := "⚫"
		if s.Current > 0 {
			status = "🟢"
		}
		lines = append(lines, fmt.Sprintf("%s %s — %d подкл, %s",
			status, name, s.Connects, FormatBytes(s.BytesTotal)))
	}

	b.send(msg.Chat.ID, "Статистика прокси:\n\n"+strings.Join(lines, "\n"))
}

func (b *Bot) cmdRevoke(msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.send(msg.Chat.ID, "Укажи пользователя: /revoke @username или /revoke 123456")
		return
	}

	telegramID, err := b.resolveUser(args)
	if err != nil {
		b.send(msg.Chat.ID, "Пользователь не найден.")
		return
	}

	count, err := b.db.DeactivateUserSecrets(telegramID)
	if err != nil {
		slog.Error("deactivate secrets", "err", err)
		b.send(msg.Chat.ID, "Ошибка.")
		return
	}

	if err := b.proxy.SyncConfig(); err != nil {
		slog.Error("sync after revoke", "err", err)
	}

	b.send(msg.Chat.ID, fmt.Sprintf("Отозвано %d сессий.", count))
	b.send(telegramID, "Твои прокси были деактивированы администратором.")
}

func (b *Bot) cmdKick(msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.send(msg.Chat.ID, "Укажи пользователя: /kick @username или /kick 123456")
		return
	}

	telegramID, err := b.resolveUser(args)
	if err != nil {
		b.send(msg.Chat.ID, "Пользователь не найден.")
		return
	}

	b.db.DeactivateUserSecrets(telegramID)
	b.db.UpdateUserStatus(telegramID, "banned")

	if err := b.proxy.SyncConfig(); err != nil {
		slog.Error("sync after kick", "err", err)
	}

	b.send(msg.Chat.ID, "Пользователь заблокирован, все сессии отозваны.")
	b.send(telegramID, "Твой доступ заблокирован.")
}

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	if cb.From.ID != b.cfg.AdminID {
		b.api.Send(tgbotapi.NewCallback(cb.ID, "Нет доступа"))
		return
	}

	parts := strings.SplitN(cb.Data, ":", 2)
	if len(parts) != 2 {
		return
	}

	action := parts[0]
	telegramID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}

	switch action {
	case "a":
		b.approveUser(cb, telegramID)
	case "d":
		b.denyUser(cb, telegramID)
	case "sa":
		b.approveSession(cb, telegramID)
	case "sd":
		b.denySession(cb, telegramID)
	}
}

func (b *Bot) approveUser(cb *tgbotapi.CallbackQuery, telegramID int64) {
	user, err := b.db.GetUserByTelegramID(telegramID)
	if err != nil {
		b.api.Send(tgbotapi.NewCallback(cb.ID, "Пользователь не найден"))
		return
	}

	if err := b.db.UpdateUserStatus(telegramID, "approved"); err != nil {
		slog.Error("update user status", "err", err)
		return
	}

	// Generate first secret.
	hexSecret, err := b.proxy.GenerateSecret()
	if err != nil {
		slog.Error("generate secret", "err", err)
		return
	}

	_, err = b.db.CreateSecret(user.ID, hexSecret, "", true)
	if err != nil {
		slog.Error("create secret", "err", err)
		return
	}

	if err := b.proxy.SyncConfig(); err != nil {
		slog.Error("sync after approve", "err", err)
	}

	link := b.proxy.ProxyLink(hexSecret)
	b.send(telegramID, fmt.Sprintf("Доступ одобрен! Вот твой прокси:\n\n%s\n\nДля доп. устройств используй /more Имя", link))

	// Update admin message.
	b.api.Send(tgbotapi.NewCallback(cb.ID, "Одобрено"))
	edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID,
		cb.Message.Text+"\n\n✅ Одобрено")
	b.api.Send(edit)
}

func (b *Bot) denyUser(cb *tgbotapi.CallbackQuery, telegramID int64) {
	b.db.UpdateUserStatus(telegramID, "banned")

	b.send(telegramID, "Заявка отклонена.")

	b.api.Send(tgbotapi.NewCallback(cb.ID, "Отклонено"))
	edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID,
		cb.Message.Text+"\n\n❌ Отклонено")
	b.api.Send(edit)
}

func (b *Bot) approveSession(cb *tgbotapi.CallbackQuery, telegramID int64) {
	secret, err := b.db.GetPendingSecretByUser(telegramID)
	if err != nil {
		b.api.Send(tgbotapi.NewCallback(cb.ID, "Запрос не найден"))
		return
	}

	if err := b.db.ActivateSecret(secret.ID); err != nil {
		slog.Error("activate secret", "err", err)
		return
	}

	if err := b.proxy.SyncConfig(); err != nil {
		slog.Error("sync after session approve", "err", err)
	}

	link := b.proxy.ProxyLink(secret.HexSecret)
	name := secret.DeviceName
	b.send(telegramID, fmt.Sprintf("Доп. сессия (%s) одобрена!\n\n%s", name, link))

	b.api.Send(tgbotapi.NewCallback(cb.ID, "Одобрено"))
	edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID,
		cb.Message.Text+"\n\n✅ Одобрено")
	b.api.Send(edit)
}

func (b *Bot) denySession(cb *tgbotapi.CallbackQuery, telegramID int64) {
	secret, err := b.db.GetPendingSecretByUser(telegramID)
	if err == nil {
		b.db.DeleteSecret(secret.ID)
	}

	b.send(telegramID, "Запрос на доп. сессию отклонён.")

	b.api.Send(tgbotapi.NewCallback(cb.ID, "Отклонено"))
	edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID,
		cb.Message.Text+"\n\n❌ Отклонено")
	b.api.Send(edit)
}

func (b *Bot) resolveUser(input string) (int64, error) {
	input = strings.TrimPrefix(input, "@")

	// Try as numeric ID first.
	if id, err := strconv.ParseInt(input, 10, 64); err == nil {
		if _, err := b.db.GetUserByTelegramID(id); err == nil {
			return id, nil
		}
	}

	// Try as username.
	var telegramID int64
	err := b.db.db.QueryRow("SELECT telegram_id FROM users WHERE username = ?", input).Scan(&telegramID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("user not found")
	}
	return telegramID, err
}

func (b *Bot) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("send message", "chat_id", chatID, "err", err)
	}
}
