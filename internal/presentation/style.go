package presentation

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiYellow  = "\x1b[33m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
)

type humanStyle struct {
	enabled bool
}

func newHumanStyle(enabled bool) humanStyle { return humanStyle{enabled: enabled} }

func (style humanStyle) section(value string) string   { return style.wrap(ansiBold, value) }
func (style humanStyle) ai(value string) string        { return style.wrap(ansiBold+ansiMagenta, value) }
func (style humanStyle) confirmed(value string) string { return style.wrap(ansiBold+ansiCyan, value) }
func (style humanStyle) action(value string) string    { return style.wrap(ansiBold, value) }
func (style humanStyle) label(value string) string     { return style.wrap(ansiBold, value) }
func (style humanStyle) failure(value string) string   { return style.wrap(ansiRed, value) }
func (style humanStyle) warning(value string) string   { return style.wrap(ansiYellow, value) }
func (style humanStyle) command(value string) string   { return style.wrap(ansiCyan, value) }
func (style humanStyle) muted(value string) string     { return style.wrap(ansiDim, value) }

func (style humanStyle) wrap(sequence, value string) string {
	if !style.enabled || value == "" {
		return value
	}

	return sequence + value + ansiReset
}
