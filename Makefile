# DEPENDENCIES:
#
# Install Go 1.16:
#
#    go get golang.org/dl/go1.16
#    go1.16 download

all: 7i.elf

7i.elf:
	env GOROOT="`go1.16 env GOROOT`" egg build -o $@

run: 7i.elf
	env GOROOT="`go1.16 env GOROOT`" egg run -p 8081:8080 7i.elf

clean:
	$(RM) 7i.elf

.PHONY: all run clean
