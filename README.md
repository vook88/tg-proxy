# tg-proxy

Личный MTProto прокси для Telegram с контролем доступа через бота.

Каждому пользователю выдаётся уникальный secret. Админ видит кто онлайн и сколько трафика через `/stats`. Можно отозвать доступ конкретному человеку.

## Как это работает

```
Пользователь → /start в боте → Админ одобряет → Уникальная прокси-ссылка
```

- **mtprotoproxy** — MTProto proxy сервер (fake-TLS, трафик выглядит как HTTPS)
- **tg-proxy** — Go бот, управляет пользователями и секретами
- **SQLite** — хранилище

## Команды бота

**Пользователи:**
- `/start` — запросить доступ к прокси
- `/my` — показать свои прокси-ссылки
- `/more Ноутбук` — запросить доп. сессию для устройства

**Админ:**
- Inline-кнопки одобрения/отклонения в уведомлениях
- `/stats` — кто онлайн, сколько трафика
- `/users` — список всех пользователей
- `/revoke @username` — отозвать все сессии
- `/reset @username` — полностью удалить пользователя
- `/kick @username` — забанить

## Установка

### Что нужно

- VPS с Ubuntu 22.04/24.04 (1 ГБ RAM хватит)
- Go 1.21+ на локальной машине (для сборки)
- Telegram бот (создать в [@BotFather](https://t.me/BotFather))
- Твой Telegram ID (узнать в [@userinfobot](https://t.me/userinfobot))

### 1. Настройка сервера

```bash
# Подключиться к серверу
ssh root@YOUR_SERVER_IP

# Установить mtprotoproxy
apt-get update && apt-get install -y python3 python3-cryptography python3-uvloop git
git clone -b stable https://github.com/alexbers/mtprotoproxy.git /opt/mtprotoproxy

# Создать директории
mkdir -p /etc/tg-proxy /var/lib/tg-proxy

# Если Telegram API не работает через IPv6 — отключить
sysctl -w net.ipv6.conf.all.disable_ipv6=1
echo 'net.ipv6.conf.all.disable_ipv6 = 1' >> /etc/sysctl.conf
```

### 2. Создать конфиг

```bash
cat > /etc/tg-proxy/env << 'EOF'
BOT_TOKEN=токен_от_BotFather
ADMIN_ID=твой_числовой_telegram_id
SERVER_HOST=IP_или_домен_сервера
SERVER_PORT=9443
DB_PATH=/var/lib/tg-proxy/data.db
CONFIG_FILE=/opt/mtprotoproxy/config.py
FAKE_TLS_HOST=cloudflare.com
RELOAD_CMD=systemctl kill -s SIGUSR2 mtprotoproxy
METRICS_URL=http://127.0.0.1:8888/
EOF
```

### 3. Собрать и задеплоить

```bash
# На локальной машине
git clone https://github.com/YOUR_USERNAME/tg-proxy.git
cd tg-proxy

# Поменять SERVER в Makefile на IP сервера
# Собрать и залить
make deploy
```

Или вручную:

```bash
# Собрать бинарник для Linux
GOOS=linux GOARCH=amd64 go build -o tg-proxy .

# Скопировать на сервер
scp tg-proxy root@SERVER:/usr/local/bin/
scp deploy/tg-proxy.service root@SERVER:/etc/systemd/system/
scp deploy/mtprotoproxy.service root@SERVER:/etc/systemd/system/

# На сервере
systemctl daemon-reload
systemctl enable tg-proxy mtprotoproxy
systemctl start tg-proxy
```

### 4. Проверить

1. Написать `/start` боту в Telegram
2. Одобрить заявку (придёт уведомление с кнопками)
3. Нажать на прокси-ссылку → "Подключить прокси"
4. `/stats` — проверить что видно подключение

## Обновление

```bash
make quick   # пересобрать + залить + рестарт
```

## Структура проекта

```
tg-proxy/
├── cmd/tg-proxy/main.go         — точка входа
├── internal/
│   ├── bot/
│   │   ├── bot.go               — обработчики команд бота
│   │   └── callbacks.go         — обработчики inline-кнопок
│   ├── config/config.go         — конфиг из env-переменных
│   ├── db/db.go                 — SQLite: пользователи и секреты
│   └── proxy/
│       ├── proxy.go             — генерация секретов, конфиг mtprotoproxy
│       └── stats.go             — парсинг Prometheus метрик
├── Makefile                     — сборка и деплой
└── deploy/
    ├── tg-proxy.service         — systemd для бота
    ├── mtprotoproxy.service     — systemd для прокси
    └── env.example              — пример конфига
```
