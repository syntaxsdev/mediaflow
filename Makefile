run: build
	@echo "Starting server 🚀"
	@set -a && . ./.env && ./media-cdn

run-air:
	@echo "Starting server with air 🚀"
	@set -a && . ./.env && air


build:
	@echo "Building server 🔨"
	@go build -o media-cdn main.go
	@echo "Server built successfully 🎉"

clean:
	@echo "Cleaning up 🧹"
	@rm -f media-cdn