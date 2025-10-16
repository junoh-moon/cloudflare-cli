PREFIX=$$HOME/.local
APP_NAME=cloudflare
APP_DIR=$(PREFIX)/$(APP_NAME)

install:
	./gradlew installDist
	rm -rf $(APP_DIR)
	mkdir -p $(APP_DIR)
	mkdir -p $(PREFIX)/bin
	cp build/install/$(APP_NAME)/bin/$(APP_NAME) $(APP_DIR)/$(APP_NAME)
	cp -r build/install/$(APP_NAME)/lib $(APP_DIR)/
	chmod +x $(APP_DIR)/$(APP_NAME)
	ln -sf $(APP_DIR)/$(APP_NAME) $(PREFIX)/bin/$(APP_NAME)

uninstall:
	rm -rf $(APP_DIR)
	rm -f $(PREFIX)/bin/$(APP_NAME)

