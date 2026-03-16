BINARY = tg-proxy
SERVER = root@YOUR_SERVER_IP
ARCH = amd64

# Cross-compile for target architecture.
build:
	GOOS=linux GOARCH=$(ARCH) go build -o $(BINARY) .

# First-time server setup: install mtg, create dirs, copy env template.
setup:
	ssh $(SERVER) "\
		mkdir -p /etc/mtg /etc/tg-proxy /var/lib/tg-proxy && \
		touch /etc/mtg/secrets.txt && \
		apt update && apt install -y wget && \
		wget -qO /usr/local/bin/mtg https://github.com/9seconds/mtg/releases/latest/download/mtg-linux-$(ARCH) && \
		chmod +x /usr/local/bin/mtg"
	scp deploy/env.example $(SERVER):/etc/tg-proxy/env.example

# Deploy binary, systemd units, enable and start services.
deploy: build
	scp $(BINARY) $(SERVER):/usr/local/bin/$(BINARY)
	scp deploy/tg-proxy.service $(SERVER):/etc/systemd/system/
	scp deploy/mtg.service $(SERVER):/etc/systemd/system/
	ssh $(SERVER) "systemctl daemon-reload && systemctl enable tg-proxy mtg && systemctl restart tg-proxy"

# Quick deploy: just binary + restart.
quick: build
	scp $(BINARY) $(SERVER):/usr/local/bin/$(BINARY)
	ssh $(SERVER) "systemctl restart tg-proxy"

.PHONY: build deploy setup quick
