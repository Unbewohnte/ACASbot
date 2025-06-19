all: clean
	mkdir -p bin
	go build && mv ACATbot* bin/

clean:
	rm -rf bin/ACATbot ACATBOT*
