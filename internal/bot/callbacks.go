package bot

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
	b.send(telegramID, fmt.Sprintf("Доп. сессия (%s) одобрена!\n\n%s", secret.DeviceName, link))

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
