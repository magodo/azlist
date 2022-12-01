package azlist

type Logger interface {
	Printf(format string, v ...any)
}

var log Logger = nullLogger{}

type nullLogger struct{}

func (nullLogger) Printf(format string, v ...any) {}

func SetLogger(l Logger) {
	log = l
}
