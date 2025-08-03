AWS_ACCESS_KEY_ID=AKIAYVF26NNM533VXIXL
AWS_SECRET_ACCESS_KEY=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
S3_BUCKET=marketplace

run: build
	@echo "Starting server 🚀"
	@AWS_ACCESS_KEY_ID=$(AWS_ACCESS_KEY_ID) AWS_SECRET_ACCESS_KEY=$(AWS_SECRET_ACCESS_KEY) S3_BUCKET=$(S3_BUCKET) ./media-cdn

run-air:
	@echo "Starting server with air 🚀"
	@AWS_ACCESS_KEY_ID=$(AWS_ACCESS_KEY_ID) AWS_SECRET_ACCESS_KEY=$(AWS_SECRET_ACCESS_KEY) S3_BUCKET=$(S3_BUCKET) air

build:
	@echo "Building server 🔨"
	@go build -o media-cdn main.go
	@echo "Server built successfully 🎉"

clean:
	@echo "Cleaning up 🧹"
	@rm -f media-cdn