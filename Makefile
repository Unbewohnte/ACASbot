all: clean
	mkdir -p bin
	go build -o ACASbot ./cmd && mv ACASbot bin/ ; cp config.json bin/ ; cp acasbot-sheet.json bin/

clean:
	rm -rf bin/ ACASbot*


cross: clean
	mkdir -p bin/ACASbot_linux_amd64 && \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/ACASbot_linux_amd64/ACASbot_linux && \
	cp COPYING README.md bin/ACASbot_linux_amd64/ 

	mkdir -p bin/ACASbot_windows_amd64 && \
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/ACASbot_windows_amd64/ACASbot_windows.exe && \
	cp COPYING README.md bin/ACASbot_windows_amd64/ 

	mkdir -p bin/ACASbot_darwin_amd64 && \
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o bin/ACASbot_darwin_amd64/ACASbot_darwin && \
	cp COPYING README.md bin/ACASbot_darwin_amd64/ 
