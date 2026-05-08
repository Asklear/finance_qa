package calc

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

var (
	ErrMissingVariable   = errors.New("missing variable")
	ErrInvalidExpression = errors.New("invalid expression")
	ErrDivisionByZero    = errors.New("division by zero")
)

// CalcExecutor evaluates arithmetic expressions with +, -, *, / and
// parentheses. Variables are resolved from the provided map.
type CalcExecutor struct{}

// NewCalcExecutor creates a reusable expression executor.
func NewCalcExecutor() *CalcExecutor {
	return &CalcExecutor{}
}

// Execute evaluates expr using vars as the variable environment.
func (e *CalcExecutor) Execute(expr string, vars map[string]float64) (float64, error) {
	p := newCalcParser(expr, vars)
	value, err := p.parseExpression()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if !p.eof() {
		return 0, invalidExpression(p.pos, "unexpected trailing token")
	}
	return value, nil
}

type calcParser struct {
	input []rune
	pos   int
	vars  map[string]float64
}

func newCalcParser(expr string, vars map[string]float64) *calcParser {
	return &calcParser{
		input: []rune(expr),
		vars:  vars,
	}
}

func (p *calcParser) eof() bool {
	return p.pos >= len(p.input)
}

func (p *calcParser) peek() rune {
	if p.eof() {
		return 0
	}
	return p.input[p.pos]
}

func (p *calcParser) next() rune {
	if p.eof() {
		return 0
	}
	ch := p.input[p.pos]
	p.pos++
	return ch
}

func (p *calcParser) skipSpaces() {
	for !p.eof() && unicode.IsSpace(p.peek()) {
		p.pos++
	}
}

func (p *calcParser) parseExpression() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}

	for {
		p.skipSpaces()
		if p.eof() {
			return left, nil
		}
		op := p.peek()
		if op != '+' && op != '-' {
			return left, nil
		}
		p.next()

		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			left += right
			continue
		}
		left -= right
	}
}

func (p *calcParser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}

	for {
		p.skipSpaces()
		if p.eof() {
			return left, nil
		}
		op := p.peek()
		if op != '*' && op != '/' {
			return left, nil
		}
		p.next()

		right, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		switch op {
		case '*':
			left *= right
		case '/':
			if right == 0 {
				return 0, divisionByZero(p.pos)
			}
			left /= right
		}
	}
}

func (p *calcParser) parseFactor() (float64, error) {
	p.skipSpaces()
	if p.eof() {
		return 0, invalidExpression(p.pos, "unexpected end of expression")
	}

	switch ch := p.peek(); {
	case ch == '+':
		p.next()
		return p.parseFactor()
	case ch == '-':
		p.next()
		value, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -value, nil
	default:
		return p.parsePrimary()
	}
}

func (p *calcParser) parsePrimary() (float64, error) {
	p.skipSpaces()
	if p.eof() {
		return 0, invalidExpression(p.pos, "unexpected end of expression")
	}

	ch := p.peek()
	switch {
	case ch == '(':
		p.next()
		value, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		p.skipSpaces()
		if p.eof() || p.peek() != ')' {
			return 0, invalidExpression(p.pos, "missing closing parenthesis")
		}
		p.next()
		return value, nil
	case isIdentStart(ch):
		name := p.readIdentifier()
		if value, ok := p.vars[name]; ok {
			return value, nil
		}
		return 0, missingVariable(name)
	case isNumberStart(ch):
		return p.readNumber()
	default:
		return 0, invalidExpression(p.pos, fmt.Sprintf("unexpected token %q", ch))
	}
}

func (p *calcParser) readIdentifier() string {
	start := p.pos
	p.next()
	for !p.eof() && isIdentPart(p.peek()) {
		p.next()
	}
	return string(p.input[start:p.pos])
}

func (p *calcParser) readNumber() (float64, error) {
	start := p.pos
	dotSeen := false
	for !p.eof() {
		ch := p.peek()
		switch {
		case unicode.IsDigit(ch):
			p.next()
		case ch == '.' && !dotSeen:
			dotSeen = true
			p.next()
		default:
			goto done
		}
	}
done:
	token := strings.TrimSpace(string(p.input[start:p.pos]))
	if token == "" || token == "." {
		return 0, invalidExpression(start, "invalid number")
	}
	value, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return 0, invalidExpression(start, fmt.Sprintf("invalid number %q", token))
	}
	return value, nil
}

func isIdentStart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func isIdentPart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

func isNumberStart(ch rune) bool {
	return unicode.IsDigit(ch) || ch == '.'
}

func missingVariable(name string) error {
	return fmt.Errorf("%w: %s", ErrMissingVariable, name)
}

func invalidExpression(pos int, msg string) error {
	return fmt.Errorf("%w at pos %d: %s", ErrInvalidExpression, pos, msg)
}

func divisionByZero(pos int) error {
	return fmt.Errorf("%w at pos %d", ErrDivisionByZero, pos)
}
