# Makefile for compiling Go Lambda function for ARM architecture

# Define variables
BINARY_NAME = bootstrap
ZIP_NAME = main.zip
GOOS = linux
GOARCH = arm64

# Default target
all: cleanall build zip

# Build target
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BINARY_NAME) main.go

# Zip target
zip: build
	zip $(ZIP_NAME) $(BINARY_NAME)

# Clean target
clean:
	rm -f $(BINARY_NAME)

# Clean target
cleanall:
	rm -f $(BINARY_NAME) $(ZIP_NAME)


