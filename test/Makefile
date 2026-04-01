# Wrote by yijian on 2020/12/25
all: test_simlog

test_simlog: test_simlog.go
ifeq ($(OS),Windows_NT)
	set GOOS=windows
	set GOARCH=amd64
	go build -o test_simlog.exe test_simlog.go
else
	go build -o test_simlog test_simlog.go
endif

install: test_simlog
ifeq ($(OS),Windows_NT)
	copy test_simlog.exe %GOPATH%\bin\
else
	cp test_simlog $$GOPATH/bin/
endif

.PHONY: clean tidy

clean:
ifeq ($(OS),Windows_NT)
	del test_simlog.exe
else
	rm -f test_simlog
endif

tidy:
	go mod tidy