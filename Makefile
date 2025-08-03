run: build
	@echo "Starting server 🚀"
	@set -a && . ./.env && ./mediaflow

run-air:
	@echo "Starting server with air 🚀"
	@set -a && . ./.env && air


build:
	@echo "Building server 🔨"
	@go build -o mediaflow main.go
	@echo "Server built successfully 🎉"

clean:
	@echo "Cleaning up 🧹"
	@rm -f mediaflow