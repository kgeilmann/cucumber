package cucumberexpressions

/*
 * text := token
 */
var textParser = func(tokens []token, current int) (int, node, error) {
	token := tokens[current]
	return 1, node{textNode, token.Start, token.End, token.Text, nil}, nil
}

/*
 * parameter := '{' + text* + '}'
 */
var parameterParser = parseBetween(
	parameterNode,
	beginParameter,
	endParameter,
	textParser,
)

/*
 * optional := '(' + option* + ')'
 * option := parameter | text
 */
var optionalParser = parseBetween(
	optionalNode,
	beginOptional,
	endOptional,
	parameterParser,
	textParser,
)

// alternation := alternative* + ( '/' + alternative* )+
var alternativeSeparatorParser = func(tokens []token, current int) (int, node, error) {
	if !lookingAt(tokens, current, alternation) {
		return 0, nullNode, nil
	}
	token := tokens[current]
	return 1, node{alternativeNode, token.Start, token.End, token.Text, nil}, nil
}

var alternativeParsers = []parser{
	alternativeSeparatorParser,
	optionalParser,
	parameterParser,
	textParser,
}

/*
 * alternation := (?<=boundary) + alternative* + ( '/' + alternative* )+ + (?=boundary)
 * boundary := whitespace | ^ | $
 * alternative: = optional | parameter | text
 */
var alternationParser = func(tokens []token, current int) (int, node, error) {
	previous := current - 1
	if !lookingAtAny(tokens, previous, startOfLine, whiteSpace) {
		return 0, nullNode, nil
	}

	consumed, subAst, err := parseTokensUntil(alternativeParsers, tokens, current, whiteSpace, endOfLine)
	if err != nil {
		return 0, nullNode, err
	}

	var contains = func(s []node, nodeType nodeType) bool {
		for _, a := range s {
			if a.NodeType == nodeType {
				return true
			}
		}
		return false
	}
	subCurrent := current + consumed
	if !contains(subAst, alternativeNode) {
		return 0, nullNode, nil
	}

	// Does not consume right hand boundary token
	start := tokens[current].Start
	end := tokens[subCurrent].Start
	return consumed, node{alternationNode, start, end, "", splitAlternatives(start, end, subAst)}, nil
}

/*
 * cucumber-expression :=  ( alternation | optional | parameter | text )*
 */
var cucumberExpressionParser = parseBetween(
	expressionNode,
	startOfLine,
	endOfLine,
	alternationParser,
	optionalParser,
	parameterParser,
	textParser,
)

func parse(expression string) (node, error) {
	tokens, err := tokenize(expression)
	if err != nil {
		return nullNode, err
	}
	consumed, ast, err := cucumberExpressionParser(tokens, 0)
	if err != nil {
		return nullNode, err
	}
	if consumed != len(tokens) {
		// Can't happen if configured properly
		return nullNode, NewCucumberExpressionError("Could not parse" + expression)
	}
	return ast, nil
}

type parser func(tokens []token, current int) (int, node, error)

func parseBetween(nodeType nodeType, beginToken tokenType, endToken tokenType, parsers ...parser) parser {
	return func(tokens []token, current int) (int, node, error) {
		if !lookingAt(tokens, current, beginToken) {
			return 0, nullNode, nil
		}

		subCurrent := current + 1
		consumed, subAst, err := parseTokensUntil(parsers, tokens, subCurrent, endToken)
		if err != nil {
			return 0, nullNode, err
		}
		subCurrent += consumed

		// endToken not found
		if !lookingAt(tokens, subCurrent, endToken) {
			return 0, nullNode, NewCucumberExpressionError("No end token")
		}
		// consumes endToken
		start := tokens[current].Start
		end := tokens[subCurrent].End
		return subCurrent + 1 - current, node{nodeType, start, end, "", subAst}, nil
	}
}

func parseTokensUntil(parsers []parser, expresion []token, startAt int, endTokens ...tokenType) (int, []node, error) {
	ast := make([]node, 0)
	current := startAt
	size := len(expresion)
	for current < size {
		if lookingAtAny(expresion, current, endTokens...) {
			break
		}
		consumed, node, err := parseToken(parsers, expresion, current)
		if err != nil {
			return 0, nil, nil
		}
		if consumed == 0 {
			// If configured correctly this will never happen
			// Keep to avoid infinite loops
			return 0, nil, NewCucumberExpressionError("No eligible parsers")
		}
		current += consumed
		ast = append(ast, node)
	}

	return current - startAt, ast, nil
}

func parseToken(parsers []parser, expresion []token, startAt int) (int, node, error) {
	for _, parser := range parsers {
		consumed, node, err := parser(expresion, startAt)
		if err != nil {
			return 0, nullNode, err
		}
		if consumed != 0 {
			return consumed, node, nil
		}
	}
	// If configured correctly this will never happen
	return 0, nullNode, NewCucumberExpressionError("No eligible parsers")
}

func lookingAtAny(tokens []token, at int, tokenTypes ...tokenType) bool {
	for _, tokenType := range tokenTypes {
		if lookingAt(tokens, at, tokenType) {
			return true
		}
	}
	return false
}

func lookingAt(tokens []token, at int, tokenType tokenType) bool {
	size := len(tokens)
	if at < 0 {
		return tokenType == startOfLine
	}
	if at >= size {
		return tokenType == endOfLine
	}
	return tokens[at].TokenType == tokenType
}

func splitAlternatives(start int, end int, alternation []node) []node {
	separators := make([]node, 0)
	alternatives := make([][]node, 0)
	alternative := make([]node, 0)
	for _, n := range alternation {
		if n.NodeType == alternativeNode {
			separators = append(separators, n)
			alternatives = append(alternatives, alternative)
			alternative = make([]node, 0)
		} else {
			alternative = append(alternative, n)
		}
	}
	alternatives = append(alternatives, alternative)

	return createAlternativeNodes(start, end, separators, alternatives)
}

func createAlternativeNodes(start int, end int, separators []node, alternatives [][]node) []node {
	nodes := make([]node, 0)
	for i, n := range alternatives {
		if i == 0 {
			rightSeparator := separators[i]
			nodes = append(nodes, node{alternativeNode, start, rightSeparator.Start, "", n})
		} else if i == len(alternatives)-1 {
			leftSeparator := separators[i-1]
			nodes = append(nodes, node{alternativeNode, leftSeparator.End, end, "", n})
		} else {
			leftSeparator := separators[i-1]
			rightSeparator := separators[i]
			nodes = append(nodes, node{alternativeNode, leftSeparator.End, rightSeparator.Start, "", n})
		}
	}
	return nodes
}