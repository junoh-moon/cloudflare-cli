PREFIX=$$HOME/.local
install:
	./gradlew installDist

	mkdir -p $(PREFIX)/bin
	mkdir -p $(PREFIX)/lib

	cp build/install/cloudflare/bin/cloudflare $(PREFIX)/bin/
	cp build/install/cloudflare/lib/* $(PREFIX)/lib/

