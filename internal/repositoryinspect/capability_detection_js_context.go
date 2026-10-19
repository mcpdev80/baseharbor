package repositoryinspect

// JavaScript interpolation, scope and declaration context helpers.

func jsInterpolationEnd(data []byte, start int) int {
	depth := 1
	var controlParens []bool
	var tokens []string // Advance lexical context once, never rescan prefixes.
	for i := start; i < len(data); {
		c := data[i]
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if c == '\n' {
			tokens = append(tokens, "\n")
			i++
			continue
		}
		if c == '/' && i+1 < len(data) && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(data) && data[i+1] == '*' {
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			if i+1 < len(data) {
				i += 2
			}
			continue
		}
		if c == 39 || c == 34 || c == 96 {
			quote := c
			i++
			for i < len(data) {
				if data[i] == '\\' && i+1 < len(data) {
					i += 2
					continue
				}
				if data[i] == quote {
					i++
					break
				}
				i++
			}
			tokens = append(tokens, "LITERAL")
			continue
		}
		if c == '/' && jsRegexCanStart(tokens) {
			i++
			inClass := false
			for i < len(data) {
				if data[i] == '\\' && i+1 < len(data) {
					i += 2
					continue
				}
				if data[i] == '[' {
					inClass = true
				}
				if data[i] == ']' {
					inClass = false
				}
				if data[i] == '/' && !inClass {
					i++
					break
				}
				i++
			}
			tokens = append(tokens, "LITERAL")
			continue
		}
		if c == '(' {
			prev := previousJSToken(tokens)
			control := false
			switch prev {
			case "if", "while", "for", "with", "switch", "catch":
				control = true
			}
			controlParens = append(controlParens, control)
			tokens = append(tokens, "(")
			i++
			continue
		}
		if c == ')' {
			kind := ")"
			if len(controlParens) > 0 {
				if controlParens[len(controlParens)-1] {
					kind = "CONTROL_CLOSE"
				}
				controlParens = controlParens[:len(controlParens)-1]
			}
			tokens = append(tokens, kind)
			i++
			continue
		}
		if c == '{' {
			depth++
			tokens = append(tokens, "{")
			i++
			continue
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return i
			}
			tokens = append(tokens, "}")
			i++
			continue
		}
		if c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			begin := i
			i++
			for i < len(data) {
				x := data[i]
				if !(x == '_' || x == '$' || x >= 'a' && x <= 'z' || x >= 'A' && x <= 'Z' || x >= '0' && x <= '9') {
					break
				}
				i++
			}
			tokens = append(tokens, string(data[begin:i]))
			continue
		}
		if c >= '0' && c <= '9' {
			for i < len(data) && (data[i] >= '0' && data[i] <= '9' || data[i] == '.') {
				i++
			}
			tokens = append(tokens, "NUMBER")
			continue
		}
		tokens = append(tokens, string(c))
		i++
	}
	return -1
}
func jsCurrentScopeStart(starts []int) int {
	if len(starts) == 0 {
		return 0
	}
	return starts[len(starts)-1]
}

func jsDeclarationBody(tokens []string, scopeStart int) bool {
	// Consider statement boundaries only in the innermost lexical brace scope.
	// An outer function declaration must not turn inner object expressions into blocks.
	start := scopeStart
	for i := scopeStart; i < len(tokens); i++ {
		if tokens[i] == ";" || tokens[i] == "BLOCK_CLOSE" {
			start = i + 1
		}
	}
	declarationAt := func(i int) bool {
		for i < len(tokens) && (tokens[i] == "\n" || tokens[i] == "export" || tokens[i] == "default" || tokens[i] == "async") {
			i++
		}
		return i < len(tokens) && (tokens[i] == "function" || tokens[i] == "class")
	}
	if declarationAt(start) {
		return true
	}
	// An ASI boundary can begin a new declaration without an explicit ';'.
	// Do not mistake expressions such as "const f =\nfunction() {}" for
	// declarations: the preceding expression must already be complete.
	for i := start + 1; i < len(tokens); i++ {
		if tokens[i] != "\n" || !declarationAt(i+1) {
			continue
		}
		if jsCanCompleteExpression(tokens, start, i) {
			return true
		}
	}
	return false
}

func jsCanCompleteExpression(tokens []string, start, end int) bool {
	// A completed expression (not a token whitelist) can terminate a simple
	// statement at a newline. Reject continuations/operators and unclosed
	// expression delimiters in this lexical statement segment.
	var parens, brackets int
	for _, tok := range tokens[start:end] {
		switch tok {
		case "(":
			parens++
		case ")":
			parens--
		case "[":
			brackets++
		case "]":
			brackets--
		}
	}
	if parens != 0 || brackets != 0 {
		return false
	}
	last := previousJSToken(tokens[:end])
	if (last == "+" || last == "-") && end >= 2 {
		prev := previousJSToken(tokens[:end-1])
		if prev == last {
			return true
		} // postfix update expression
	}
	if last == "" {
		return false
	}
	switch last {
	case "=", "+", "-", "*", "/", "%", "!", "?", ":", ",", ".", "&", "|", "^", "~", ">", "<", "(", "[", "{", "return", "throw", "new", "const", "let", "var", "function", "class", "=>", "in", "instanceof", "await", "yield", "as", "satisfies":
		return false
	}
	// Identifiers, literal values, completed access/call/index expressions,
	// and postfix updates terminate a statement. Earlier operators may still
	// occur inside an otherwise completed expression.
	return true
}
