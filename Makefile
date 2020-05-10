.PHONY: all
all: test

test:
	go get -v github.com/jstemmer/go-junit-report
	go build -o go-junit-report github.com/jstemmer/go-junit-report
	go get -v
	go test -v -run=Test_Unit 2>&1 | ./go-junit-report > report.xml

update:
	GOPRIVATE="evalgo.org/evmsg,evalgo.org/eve" go get -u -v

build: update
	GOOS=linux GOARCH=amd64 go build -o files.linux.amd64 cmd/service/main.go
	GOOS=darwin GOARCH=amd64 go build -o files.darwin.amd64 cmd/service/main.go
	GOOS=windows GOARCH=amd64 go build -o files.windows.amd64 cmd/service/main.go

.PHONY: clean 
clean:
	find . -name "*~" | xargs rm -fv
	rm -fv go-junit-report report.xml
	rm -fv files.*.amd64

