package prompt

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Executor func(*Buffer) string
type Completer func(*Buffer) []string

type Prompt struct {
	in        ConsoleParser
	buf       *Buffer
	renderer  *Render
	executor  Executor
	completer Completer
	chosen    int  // -1 means nothing one is chosen.
}

func (p *Prompt) Run() {
	p.setUp()
	defer p.tearDown()

	bufCh := make(chan []byte, 128)
	go readBuffer(bufCh)

	exitCh := make(chan bool, 16)
	winSizeCh := make(chan *WinSize, 128)
	go handleSignals(p.in, exitCh, winSizeCh)

	for {
		select {
		case b := <-bufCh:
			p.renderer.Erase(p.buf)
			ac := p.in.GetASCIICode(b)
			if ac == nil {
				if p.chosen != -1 {
					c := p.completer(p.buf)[p.chosen]
					w := p.buf.Document().GetWordBeforeCursor()
					if w != "" {
						p.buf.DeleteBeforeCursor(len([]rune(w)))
					}
					p.buf.InsertText(c, false, true)
				}
				p.chosen = -1
				p.buf.InsertText(string(b), false, true)
			} else if ac.Key == ControlJ || ac.Key == Enter {
				if p.chosen != -1 {
					c := p.completer(p.buf)[p.chosen]
					w := p.buf.Document().GetWordBeforeCursor()
					if w != "" {
						p.buf.DeleteBeforeCursor(len([]rune(w)))
					}
					p.buf.InsertText(c, false, true)
				}
				res := p.executor(p.buf)
				p.renderer.BreakLine(p.buf, res)
				p.buf = NewBuffer()
				p.chosen = -1
			} else if ac.Key == ControlC || ac.Key == ControlD {
				return
			} else if ac.Key == BackTab || ac.Key == Up {
				p.chosen -= 1
			} else if ac.Key == Tab || ac.Key == ControlI || ac.Key == Down {
				p.chosen += 1
			} else {
				InputHandler(ac, p.buf)
				p.chosen = -1
			}

			completions := p.completer(p.buf)
			if p.chosen >= len(completions) {
				p.chosen = -1
			} else if p.chosen < -1 {
				p.chosen = len(completions) - 1
			}
			p.renderer.Render(p.buf, completions, p.chosen)
		case w := <-winSizeCh:
			p.renderer.UpdateWinSize(w)
			p.renderer.Erase(p.buf)
			completions := p.completer(p.buf)
			p.renderer.Render(p.buf, completions, p.chosen)
		case e := <-exitCh:
			if e {
				return
			}
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (p *Prompt) setUp() {
	p.in.Setup()
	p.renderer.Setup()
	p.renderer.UpdateWinSize(p.in.GetWinSize())
}

func (p *Prompt) tearDown() {
	p.in.TearDown()
	p.renderer.TearDown()
}

func readBuffer(bufCh chan []byte) {
	buf := make([]byte, 1024)

	for {
		if n, err := syscall.Read(syscall.Stdin, buf); err == nil {
			bufCh <- buf[:n]
		}
	}
}

func handleSignals(in ConsoleParser, exitCh chan bool, winSizeCh chan *WinSize) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(
		sigCh,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGWINCH,
	)

	for {
		s := <-sigCh
		switch s {
		// kill -SIGHUP XXXX
		case syscall.SIGHUP:
			exitCh <- true

			// kill -SIGINT XXXX or Ctrl+c
		case syscall.SIGINT:
			exitCh <- true

			// kill -SIGTERM XXXX
		case syscall.SIGTERM:
			exitCh <- true

			// kill -SIGQUIT XXXX
		case syscall.SIGQUIT:
			exitCh <- true

		case syscall.SIGWINCH:
			winSizeCh <- in.GetWinSize()
		default:
		}
	}
}