BINARY = tg-proxy
SERVER = root@194.147.114.31
ARCH = amd64

# Cross-compile for target architecture.
build:
	GOOS=linux GOARCH=$(ARCH) go build -o $(BINARY) .

# First-time server setup: install mtprotoproxy, create dirs, copy env template.
setup:
	ssh $(SERVER) "\
		mkdir -p /etc/tg-proxy /var/lib/tg-proxy && \
		apt-get update -qq && \
		apt-get install -y -qq python3 python3-cryptography python3-uvloop git && \
		git clone -b stable https://github.com/alexbers/mtprotoproxy.git /opt/mtprotoproxy || true"
	scp deploy/env.example $(SERVER):/etc/tg-proxy/env.example

# Deploy binary, systemd units, enable and start services.
deploy: build
	scp $(BINARY) $(SERVER):/usr/local/bin/$(BINARY)
	scp deploy/tg-proxy.service $(SERVER):/etc/systemd/system/
	scp deploy/mtprotoproxy.service $(SERVER):/etc/systemd/system/
	ssh $(SERVER) "systemctl daemon-reload && systemctl enable tg-proxy mtprotoproxy && systemctl restart tg-proxy"

# Quick deploy: just binary + restart.
quick: build
	scp $(BINARY) $(SERVER):/usr/local/bin/$(BINARY)
	ssh $(SERVER) "systemctl restart tg-proxy"

.PHONY: build deploy setup quick
