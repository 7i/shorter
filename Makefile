all: 7i.elf

7i.elf:
	egg build -o $@

run: 7i.elf
	egg run -p 8081:8080 7i.elf

clean:
	$(RM) 7i.elf

.PHONY: all run clean
