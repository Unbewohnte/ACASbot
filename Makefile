all: clean
	mkdir -p bin
	go build && mv ACASbot* bin/

clean:
	rm -rf bin/ACASbot ACASBOT*
