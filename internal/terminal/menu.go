package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/Innocent-children/taskbelay/internal/platform"
	"golang.org/x/term"
)

var ErrBack = errors.New("返回")

const (
	keyUp = 256 + iota
	keyDown
	keyLeft
	keyRight
)

type inputByte struct {
	value byte
	err   error
}

type Menu struct {
	input   *os.File
	output  *os.File
	bytes   chan inputByte
	done    chan struct{}
	restore func()
}

func Available(input *os.File, output io.Writer) bool {
	f, ok := output.(*os.File)
	return ok && term.IsTerminal(int(input.Fd())) && term.IsTerminal(int(f.Fd())) && os.Getenv("TERM") != "dumb"
}

func Open(input *os.File, output io.Writer) (*Menu, error) {
	if !Available(input, output) {
		return nil, fmt.Errorf("菜单需要交互终端；请在终端运行 taskbelay，或用 taskbelay --help 查看子命令")
	}
	out := output.(*os.File)
	restore, err := platform.EnableTerminalOutput(out)
	if err != nil {
		return nil, err
	}
	m := &Menu{input: input, output: out, bytes: make(chan inputByte), done: make(chan struct{}), restore: restore}
	go func() {
		var b [1]byte
		for {
			_, err := io.ReadFull(input, b[:])
			select {
			case m.bytes <- inputByte{b[0], err}:
			case <-m.done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return m, nil
}

/**
 * 标准输入属于调用方；部分终端无法通过 Close 中断正在进行的读取。
 * 读协程在下一次读取返回时结束，CLI 退出时也会随进程回收。
 */
func (m *Menu) Close() {
	close(m.done)
	m.restore()
}

func (m *Menu) readByte(ctx context.Context) (byte, error) {
	select {
	case value := <-m.bytes:
		return value.value, value.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (m *Menu) readKey(ctx context.Context) (int, error) {
	b, err := m.readByte(ctx)
	if err != nil || b != 27 {
		return int(b), err
	}
	sequence, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	b, err = m.readByte(sequence)
	if errors.Is(err, context.DeadlineExceeded) {
		return keyLeft, nil
	}
	if err != nil {
		return 0, err
	}
	if b != '[' && b != 'O' {
		return int(b), nil
	}
	for {
		b, err = m.readByte(sequence)
		if errors.Is(err, context.DeadlineExceeded) {
			return keyLeft, nil
		}
		if err != nil {
			return 0, err
		}
		if b >= 0x40 && b <= 0x7e {
			switch b {
			case 'A':
				return keyUp, nil
			case 'B':
				return keyDown, nil
			case 'C':
				return keyRight, nil
			case 'D':
				return keyLeft, nil
			}
			return 0, nil
		}
	}
}

func (m *Menu) Select(ctx context.Context, title string, items []string, multiple bool) (selected []int, resultErr error) {
	if len(items) == 0 {
		return nil, ErrBack
	}
	state, err := term.MakeRaw(int(m.input.Fd()))
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, term.Restore(int(m.input.Fd()), state)) }()
	fmt.Fprint(m.output, "\x1b[?1049h\x1b[?25l")
	defer fmt.Fprint(m.output, "\x1b[?25h\x1b[?1049l")
	cursor := 0
	checked := make([]bool, len(items))
	for {
		_, height, _ := term.GetSize(int(m.output.Fd()))
		visible := max(1, height-6)
		start := max(0, cursor-visible+1)
		var view strings.Builder
		fmt.Fprintf(&view, "\x1b[H\x1b[2J%s\r\n\r\n", safeText(title))
		for i := start; i < len(items) && i < start+visible; i++ {
			pointer, mark := "  ", ""
			if i == cursor {
				pointer = "> "
			}
			if multiple {
				mark = "[ ] "
				if checked[i] {
					mark = "[x] "
				}
			}
			fmt.Fprintf(&view, "%s%s%s\r\n", pointer, mark, safeText(items[i]))
		}
		fmt.Fprint(&view, "\r\n↑↓ 选择  →/回车 确认  ←/Esc/q 返回")
		if multiple {
			fmt.Fprint(&view, "  空格 勾选")
		}
		fmt.Fprint(&view, "\r\nCtrl+C 退出\r\n")
		if _, err := io.WriteString(m.output, view.String()); err != nil {
			return nil, err
		}
		key, err := m.readKey(ctx)
		if err != nil {
			return nil, err
		}
		switch key {
		case keyUp:
			cursor = (cursor + len(items) - 1) % len(items)
		case keyDown:
			cursor = (cursor + 1) % len(items)
		case keyLeft, 'q', 'Q':
			return nil, ErrBack
		case 3, 4:
			return nil, context.Canceled
		case ' ':
			if multiple {
				checked[cursor] = !checked[cursor]
			}
		case '\r', '\n', keyRight:
			if !multiple {
				return []int{cursor}, nil
			}
			for i, value := range checked {
				if value {
					selected = append(selected, i)
				}
			}
			return selected, nil
		}
	}
}

func (m *Menu) Confirm(ctx context.Context, title string) (bool, error) {
	choice, err := m.Select(ctx, title, []string{"取消", "确认"}, false)
	if errors.Is(err, ErrBack) {
		return false, nil
	}
	return err == nil && choice[0] == 1, err
}

func (m *Menu) Pause(ctx context.Context) (resultErr error) {
	state, err := term.MakeRaw(int(m.input.Fd()))
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, term.Restore(int(m.input.Fd()), state)) }()
	fmt.Fprint(m.output, "\r\n按回车返回菜单。\r\n")
	for {
		key, err := m.readKey(ctx)
		if err != nil {
			return err
		}
		if key == 3 || key == 4 {
			return context.Canceled
		}
		if key == '\r' || key == '\n' || key == keyLeft || key == 'q' {
			return nil
		}
	}
}

func safeText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}
