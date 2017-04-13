package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

const endSymbol rune = 1114112

/* The rule types inferred from the grammar are below. */
type pegRule uint8

const (
	ruleUnknown pegRule = iota
	ruleroot
	ruleEOT
	ruleexpression
	ruletitle_info
	ruletable_info
	rulecomment
	ruleempty_line
	ruletable_name_info
	rulecolumn_info
	rulecolumn_attribute
	rulerelation
	rulecolumn_comment
	ruleindex_info
	ruletitle
	rulecomment_string
	rulewhitespace
	rulenewline
	rulespace
	rulenotnull
	ruleunique
	rulereal_table_name
	ruletable_name
	rulereal_column_name
	rulecolumn_name
	rulerelation_point
	rulepkey
	rulecol_type
	ruledefault
	rulecardinality_right
	rulecardinality_left
	rulecardinality
	rulePegText
	ruleAction0
	ruleAction1
	ruleAction2
	ruleAction3
	ruleAction4
	ruleAction5
	ruleAction6
	ruleAction7
	ruleAction8
	ruleAction9
	ruleAction10
	ruleAction11
	ruleAction12
	ruleAction13
	ruleAction14
	ruleAction15
	ruleAction16
	ruleAction17
	ruleAction18
	ruleAction19
)

var rul3s = [...]string{
	"Unknown",
	"root",
	"EOT",
	"expression",
	"title_info",
	"table_info",
	"comment",
	"empty_line",
	"table_name_info",
	"column_info",
	"column_attribute",
	"relation",
	"column_comment",
	"index_info",
	"title",
	"comment_string",
	"whitespace",
	"newline",
	"space",
	"notnull",
	"unique",
	"real_table_name",
	"table_name",
	"real_column_name",
	"column_name",
	"relation_point",
	"pkey",
	"col_type",
	"default",
	"cardinality_right",
	"cardinality_left",
	"cardinality",
	"PegText",
	"Action0",
	"Action1",
	"Action2",
	"Action3",
	"Action4",
	"Action5",
	"Action6",
	"Action7",
	"Action8",
	"Action9",
	"Action10",
	"Action11",
	"Action12",
	"Action13",
	"Action14",
	"Action15",
	"Action16",
	"Action17",
	"Action18",
	"Action19",
}

type token32 struct {
	pegRule
	begin, end uint32
}

func (t *token32) String() string {
	return fmt.Sprintf("\x1B[34m%v\x1B[m %v %v", rul3s[t.pegRule], t.begin, t.end)
}

type node32 struct {
	token32
	up, next *node32
}

func (node *node32) Print(buffer string) {
	var print func(node *node32, depth int)
	print = func(node *node32, depth int) {
		for node != nil {
			for c := 0; c < depth; c++ {
				fmt.Printf(" ")
			}
			fmt.Printf("\x1B[34m%v\x1B[m %v\n", rul3s[node.pegRule], strconv.Quote(string(([]rune(buffer)[node.begin:node.end]))))
			if node.up != nil {
				print(node.up, depth+1)
			}
			node = node.next
		}
	}
	print(node, 0)
}

type tokens32 struct {
	tree []token32
}

func (t *tokens32) Trim(length uint32) {
	t.tree = t.tree[:length]
}

func (t *tokens32) Print() {
	for _, token := range t.tree {
		fmt.Println(token.String())
	}
}

func (t *tokens32) AST() *node32 {
	type element struct {
		node *node32
		down *element
	}
	tokens := t.Tokens()
	var stack *element
	for _, token := range tokens {
		if token.begin == token.end {
			continue
		}
		node := &node32{token32: token}
		for stack != nil && stack.node.begin >= token.begin && stack.node.end <= token.end {
			stack.node.next = node.up
			node.up = stack.node
			stack = stack.down
		}
		stack = &element{node: node, down: stack}
	}
	if stack != nil {
		return stack.node
	}
	return nil
}

func (t *tokens32) PrintSyntaxTree(buffer string) {
	t.AST().Print(buffer)
}

func (t *tokens32) Add(rule pegRule, begin, end, index uint32) {
	if tree := t.tree; int(index) >= len(tree) {
		expanded := make([]token32, 2*len(tree))
		copy(expanded, tree)
		t.tree = expanded
	}
	t.tree[index] = token32{
		pegRule: rule,
		begin:   begin,
		end:     end,
	}
}

func (t *tokens32) Tokens() []token32 {
	return t.tree
}

type Parser struct {
	ErdM

	Buffer string
	buffer []rune
	rules  [53]func() bool
	parse  func(rule ...int) error
	reset  func()
	Pretty bool
	tokens32
}

func (p *Parser) Parse(rule ...int) error {
	return p.parse(rule...)
}

func (p *Parser) Reset() {
	p.reset()
}

type textPosition struct {
	line, symbol int
}

type textPositionMap map[int]textPosition

func translatePositions(buffer []rune, positions []int) textPositionMap {
	length, translations, j, line, symbol := len(positions), make(textPositionMap, len(positions)), 0, 1, 0
	sort.Ints(positions)

search:
	for i, c := range buffer {
		if c == '\n' {
			line, symbol = line+1, 0
		} else {
			symbol++
		}
		if i == positions[j] {
			translations[positions[j]] = textPosition{line, symbol}
			for j++; j < length; j++ {
				if i != positions[j] {
					continue search
				}
			}
			break search
		}
	}

	return translations
}

type parseError struct {
	p   *Parser
	max token32
}

func (e *parseError) Error() string {
	tokens, error := []token32{e.max}, "\n"
	positions, p := make([]int, 2*len(tokens)), 0
	for _, token := range tokens {
		positions[p], p = int(token.begin), p+1
		positions[p], p = int(token.end), p+1
	}
	translations := translatePositions(e.p.buffer, positions)
	format := "parse error near %v (line %v symbol %v - line %v symbol %v):\n%v\n"
	if e.p.Pretty {
		format = "parse error near \x1B[34m%v\x1B[m (line %v symbol %v - line %v symbol %v):\n%v\n"
	}
	for _, token := range tokens {
		begin, end := int(token.begin), int(token.end)
		error += fmt.Sprintf(format,
			rul3s[token.pegRule],
			translations[begin].line, translations[begin].symbol,
			translations[end].line, translations[end].symbol,
			strconv.Quote(string(e.p.buffer[begin:end])))
	}

	return error
}

func (p *Parser) PrintSyntaxTree() {
	p.tokens32.PrintSyntaxTree(p.Buffer)
}

func (p *Parser) Execute() {
	buffer, _buffer, text, begin, end := p.Buffer, p.buffer, "", 0, 0
	for _, token := range p.Tokens() {
		switch token.pegRule {

		case rulePegText:
			begin, end = int(token.begin), int(token.end)
			text = string(_buffer[begin:end])

		case ruleAction0:
			p.Err(begin, buffer)
		case ruleAction1:
			p.Err(begin, buffer)
		case ruleAction2:
			p.setTitle(text)
		case ruleAction3:
			p.addTableTitleReal(text)
		case ruleAction4:
			p.addTableTitle(text)
		case ruleAction5:
			p.addPrimaryKey(text)
		case ruleAction6:
			p.setColumnNameReal(text)
		case ruleAction7:
			p.setColumnName(text)
		case ruleAction8:
			p.addColumnType(text)
		case ruleAction9:
			p.setNotNull()
		case ruleAction10:
			p.setUnique()
		case ruleAction11:
			p.setColumnDefault(text)
		case ruleAction12:
			p.setRelationSource(text)
		case ruleAction13:
			p.setRelationDestination(text)
		case ruleAction14:
			p.setRelationTableNameReal(text)
		case ruleAction15:
			p.addComment(text)
		case ruleAction16:
			p.setIndexName(text)
		case ruleAction17:
			p.setIndexColumn(text)
		case ruleAction18:
			p.setIndexColumn(text)
		case ruleAction19:
			p.setUniqueIndex()

		}
	}
	_, _, _, _, _ = buffer, _buffer, text, begin, end
}

func (p *Parser) Init() {
	var (
		max                  token32
		position, tokenIndex uint32
		buffer               []rune
	)
	p.reset = func() {
		max = token32{}
		position, tokenIndex = 0, 0

		p.buffer = []rune(p.Buffer)
		if len(p.buffer) == 0 || p.buffer[len(p.buffer)-1] != endSymbol {
			p.buffer = append(p.buffer, endSymbol)
		}
		buffer = p.buffer
	}
	p.reset()

	_rules, tree := p.rules, tokens32{tree: make([]token32, math.MaxInt16)}
	p.parse = func(rule ...int) error {
		r := 1
		if len(rule) > 0 {
			r = rule[0]
		}
		matches := p.rules[r]()
		p.tokens32 = tree
		if matches {
			p.Trim(tokenIndex)
			return nil
		}
		return &parseError{p, max}
	}

	add := func(rule pegRule, begin uint32) {
		tree.Add(rule, begin, position, tokenIndex)
		tokenIndex++
		if begin != position && position > max.end {
			max = token32{rule, begin, position}
		}
	}

	matchDot := func() bool {
		if buffer[position] != endSymbol {
			position++
			return true
		}
		return false
	}

	/*matchChar := func(c byte) bool {
		if buffer[position] == c {
			position++
			return true
		}
		return false
	}*/

	/*matchRange := func(lower byte, upper byte) bool {
		if c := buffer[position]; c >= lower && c <= upper {
			position++
			return true
		}
		return false
	}*/

	_rules = [...]func() bool{
		nil,
		/* 0 root <- <((expression EOT) / (expression <.+> Action0 EOT) / (<.+> Action1 EOT))> */
		func() bool {
			position0, tokenIndex0 := position, tokenIndex
			{
				position1 := position
				{
					position2, tokenIndex2 := position, tokenIndex
					if !_rules[ruleexpression]() {
						goto l3
					}
					if !_rules[ruleEOT]() {
						goto l3
					}
					goto l2
				l3:
					position, tokenIndex = position2, tokenIndex2
					if !_rules[ruleexpression]() {
						goto l4
					}
					{
						position5 := position
						if !matchDot() {
							goto l4
						}
					l6:
						{
							position7, tokenIndex7 := position, tokenIndex
							if !matchDot() {
								goto l7
							}
							goto l6
						l7:
							position, tokenIndex = position7, tokenIndex7
						}
						add(rulePegText, position5)
					}
					if !_rules[ruleAction0]() {
						goto l4
					}
					if !_rules[ruleEOT]() {
						goto l4
					}
					goto l2
				l4:
					position, tokenIndex = position2, tokenIndex2
					{
						position8 := position
						if !matchDot() {
							goto l0
						}
					l9:
						{
							position10, tokenIndex10 := position, tokenIndex
							if !matchDot() {
								goto l10
							}
							goto l9
						l10:
							position, tokenIndex = position10, tokenIndex10
						}
						add(rulePegText, position8)
					}
					if !_rules[ruleAction1]() {
						goto l0
					}
					if !_rules[ruleEOT]() {
						goto l0
					}
				}
			l2:
				add(ruleroot, position1)
			}
			return true
		l0:
			position, tokenIndex = position0, tokenIndex0
			return false
		},
		/* 1 EOT <- <!.> */
		func() bool {
			position11, tokenIndex11 := position, tokenIndex
			{
				position12 := position
				{
					position13, tokenIndex13 := position, tokenIndex
					if !matchDot() {
						goto l13
					}
					goto l11
				l13:
					position, tokenIndex = position13, tokenIndex13
				}
				add(ruleEOT, position12)
			}
			return true
		l11:
			position, tokenIndex = position11, tokenIndex11
			return false
		},
		/* 2 expression <- <(title_info (table_info / comment / empty_line)*)> */
		func() bool {
			position14, tokenIndex14 := position, tokenIndex
			{
				position15 := position
				if !_rules[ruletitle_info]() {
					goto l14
				}
			l16:
				{
					position17, tokenIndex17 := position, tokenIndex
					{
						position18, tokenIndex18 := position, tokenIndex
						if !_rules[ruletable_info]() {
							goto l19
						}
						goto l18
					l19:
						position, tokenIndex = position18, tokenIndex18
						if !_rules[rulecomment]() {
							goto l20
						}
						goto l18
					l20:
						position, tokenIndex = position18, tokenIndex18
						if !_rules[ruleempty_line]() {
							goto l17
						}
					}
				l18:
					goto l16
				l17:
					position, tokenIndex = position17, tokenIndex17
				}
				add(ruleexpression, position15)
			}
			return true
		l14:
			position, tokenIndex = position14, tokenIndex14
			return false
		},
		/* 3 title_info <- <('#' space* ('T' 'i' 't' 'l' 'e' ':') space* <title> Action2 newline)> */
		func() bool {
			position21, tokenIndex21 := position, tokenIndex
			{
				position22 := position
				if buffer[position] != rune('#') {
					goto l21
				}
				position++
			l23:
				{
					position24, tokenIndex24 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l24
					}
					goto l23
				l24:
					position, tokenIndex = position24, tokenIndex24
				}
				if buffer[position] != rune('T') {
					goto l21
				}
				position++
				if buffer[position] != rune('i') {
					goto l21
				}
				position++
				if buffer[position] != rune('t') {
					goto l21
				}
				position++
				if buffer[position] != rune('l') {
					goto l21
				}
				position++
				if buffer[position] != rune('e') {
					goto l21
				}
				position++
				if buffer[position] != rune(':') {
					goto l21
				}
				position++
			l25:
				{
					position26, tokenIndex26 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l26
					}
					goto l25
				l26:
					position, tokenIndex = position26, tokenIndex26
				}
				{
					position27 := position
					if !_rules[ruletitle]() {
						goto l21
					}
					add(rulePegText, position27)
				}
				if !_rules[ruleAction2]() {
					goto l21
				}
				if !_rules[rulenewline]() {
					goto l21
				}
				add(ruletitle_info, position22)
			}
			return true
		l21:
			position, tokenIndex = position21, tokenIndex21
			return false
		},
		/* 4 table_info <- <(table_name_info column_info* index_info*)> */
		func() bool {
			position28, tokenIndex28 := position, tokenIndex
			{
				position29 := position
				if !_rules[ruletable_name_info]() {
					goto l28
				}
			l30:
				{
					position31, tokenIndex31 := position, tokenIndex
					if !_rules[rulecolumn_info]() {
						goto l31
					}
					goto l30
				l31:
					position, tokenIndex = position31, tokenIndex31
				}
			l32:
				{
					position33, tokenIndex33 := position, tokenIndex
					if !_rules[ruleindex_info]() {
						goto l33
					}
					goto l32
				l33:
					position, tokenIndex = position33, tokenIndex33
				}
				add(ruletable_info, position29)
			}
			return true
		l28:
			position, tokenIndex = position28, tokenIndex28
			return false
		},
		/* 5 comment <- <(space* ('/' '/') comment_string newline)> */
		func() bool {
			position34, tokenIndex34 := position, tokenIndex
			{
				position35 := position
			l36:
				{
					position37, tokenIndex37 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l37
					}
					goto l36
				l37:
					position, tokenIndex = position37, tokenIndex37
				}
				if buffer[position] != rune('/') {
					goto l34
				}
				position++
				if buffer[position] != rune('/') {
					goto l34
				}
				position++
				if !_rules[rulecomment_string]() {
					goto l34
				}
				if !_rules[rulenewline]() {
					goto l34
				}
				add(rulecomment, position35)
			}
			return true
		l34:
			position, tokenIndex = position34, tokenIndex34
			return false
		},
		/* 6 empty_line <- <whitespace> */
		func() bool {
			position38, tokenIndex38 := position, tokenIndex
			{
				position39 := position
				if !_rules[rulewhitespace]() {
					goto l38
				}
				add(ruleempty_line, position39)
			}
			return true
		l38:
			position, tokenIndex = position38, tokenIndex38
			return false
		},
		/* 7 table_name_info <- <(<real_table_name> Action3 space* ('/' space* <table_name> Action4) space* newline*)> */
		func() bool {
			position40, tokenIndex40 := position, tokenIndex
			{
				position41 := position
				{
					position42 := position
					if !_rules[rulereal_table_name]() {
						goto l40
					}
					add(rulePegText, position42)
				}
				if !_rules[ruleAction3]() {
					goto l40
				}
			l43:
				{
					position44, tokenIndex44 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l44
					}
					goto l43
				l44:
					position, tokenIndex = position44, tokenIndex44
				}
				if buffer[position] != rune('/') {
					goto l40
				}
				position++
			l45:
				{
					position46, tokenIndex46 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l46
					}
					goto l45
				l46:
					position, tokenIndex = position46, tokenIndex46
				}
				{
					position47 := position
					if !_rules[ruletable_name]() {
						goto l40
					}
					add(rulePegText, position47)
				}
				if !_rules[ruleAction4]() {
					goto l40
				}
			l48:
				{
					position49, tokenIndex49 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l49
					}
					goto l48
				l49:
					position, tokenIndex = position49, tokenIndex49
				}
			l50:
				{
					position51, tokenIndex51 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l51
					}
					goto l50
				l51:
					position, tokenIndex = position51, tokenIndex51
				}
				add(ruletable_name_info, position41)
			}
			return true
		l40:
			position, tokenIndex = position40, tokenIndex40
			return false
		},
		/* 8 column_info <- <(column_attribute (space* relation (space* relation)*)? (newline? column_comment)* newline?)> */
		func() bool {
			position52, tokenIndex52 := position, tokenIndex
			{
				position53 := position
				if !_rules[rulecolumn_attribute]() {
					goto l52
				}
				{
					position54, tokenIndex54 := position, tokenIndex
				l56:
					{
						position57, tokenIndex57 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l57
						}
						goto l56
					l57:
						position, tokenIndex = position57, tokenIndex57
					}
					if !_rules[rulerelation]() {
						goto l54
					}
				l58:
					{
						position59, tokenIndex59 := position, tokenIndex
					l60:
						{
							position61, tokenIndex61 := position, tokenIndex
							if !_rules[rulespace]() {
								goto l61
							}
							goto l60
						l61:
							position, tokenIndex = position61, tokenIndex61
						}
						if !_rules[rulerelation]() {
							goto l59
						}
						goto l58
					l59:
						position, tokenIndex = position59, tokenIndex59
					}
					goto l55
				l54:
					position, tokenIndex = position54, tokenIndex54
				}
			l55:
			l62:
				{
					position63, tokenIndex63 := position, tokenIndex
					{
						position64, tokenIndex64 := position, tokenIndex
						if !_rules[rulenewline]() {
							goto l64
						}
						goto l65
					l64:
						position, tokenIndex = position64, tokenIndex64
					}
				l65:
					if !_rules[rulecolumn_comment]() {
						goto l63
					}
					goto l62
				l63:
					position, tokenIndex = position63, tokenIndex63
				}
				{
					position66, tokenIndex66 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l66
					}
					goto l67
				l66:
					position, tokenIndex = position66, tokenIndex66
				}
			l67:
				add(rulecolumn_info, position53)
			}
			return true
		l52:
			position, tokenIndex = position52, tokenIndex52
			return false
		},
		/* 9 column_attribute <- <(space+ (<pkey> Action5)? <real_column_name> Action6 ('/' <column_name> Action7)? space+ '[' <col_type> Action8 ']' (('[' notnull Action9 ']') / ('[' unique Action10 ']'))* ('[' '=' <default> Action11 ']')? newline?)> */
		func() bool {
			position68, tokenIndex68 := position, tokenIndex
			{
				position69 := position
				if !_rules[rulespace]() {
					goto l68
				}
			l70:
				{
					position71, tokenIndex71 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l71
					}
					goto l70
				l71:
					position, tokenIndex = position71, tokenIndex71
				}
				{
					position72, tokenIndex72 := position, tokenIndex
					{
						position74 := position
						if !_rules[rulepkey]() {
							goto l72
						}
						add(rulePegText, position74)
					}
					if !_rules[ruleAction5]() {
						goto l72
					}
					goto l73
				l72:
					position, tokenIndex = position72, tokenIndex72
				}
			l73:
				{
					position75 := position
					if !_rules[rulereal_column_name]() {
						goto l68
					}
					add(rulePegText, position75)
				}
				if !_rules[ruleAction6]() {
					goto l68
				}
				{
					position76, tokenIndex76 := position, tokenIndex
					if buffer[position] != rune('/') {
						goto l76
					}
					position++
					{
						position78 := position
						if !_rules[rulecolumn_name]() {
							goto l76
						}
						add(rulePegText, position78)
					}
					if !_rules[ruleAction7]() {
						goto l76
					}
					goto l77
				l76:
					position, tokenIndex = position76, tokenIndex76
				}
			l77:
				if !_rules[rulespace]() {
					goto l68
				}
			l79:
				{
					position80, tokenIndex80 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l80
					}
					goto l79
				l80:
					position, tokenIndex = position80, tokenIndex80
				}
				if buffer[position] != rune('[') {
					goto l68
				}
				position++
				{
					position81 := position
					if !_rules[rulecol_type]() {
						goto l68
					}
					add(rulePegText, position81)
				}
				if !_rules[ruleAction8]() {
					goto l68
				}
				if buffer[position] != rune(']') {
					goto l68
				}
				position++
			l82:
				{
					position83, tokenIndex83 := position, tokenIndex
					{
						position84, tokenIndex84 := position, tokenIndex
						if buffer[position] != rune('[') {
							goto l85
						}
						position++
						if !_rules[rulenotnull]() {
							goto l85
						}
						if !_rules[ruleAction9]() {
							goto l85
						}
						if buffer[position] != rune(']') {
							goto l85
						}
						position++
						goto l84
					l85:
						position, tokenIndex = position84, tokenIndex84
						if buffer[position] != rune('[') {
							goto l83
						}
						position++
						if !_rules[ruleunique]() {
							goto l83
						}
						if !_rules[ruleAction10]() {
							goto l83
						}
						if buffer[position] != rune(']') {
							goto l83
						}
						position++
					}
				l84:
					goto l82
				l83:
					position, tokenIndex = position83, tokenIndex83
				}
				{
					position86, tokenIndex86 := position, tokenIndex
					if buffer[position] != rune('[') {
						goto l86
					}
					position++
					if buffer[position] != rune('=') {
						goto l86
					}
					position++
					{
						position88 := position
						if !_rules[ruledefault]() {
							goto l86
						}
						add(rulePegText, position88)
					}
					if !_rules[ruleAction11]() {
						goto l86
					}
					if buffer[position] != rune(']') {
						goto l86
					}
					position++
					goto l87
				l86:
					position, tokenIndex = position86, tokenIndex86
				}
			l87:
				{
					position89, tokenIndex89 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l89
					}
					goto l90
				l89:
					position, tokenIndex = position89, tokenIndex89
				}
			l90:
				add(rulecolumn_attribute, position69)
			}
			return true
		l68:
			position, tokenIndex = position68, tokenIndex68
			return false
		},
		/* 10 relation <- <((<cardinality_left> Action12)? space* ('-' '-') space* (<cardinality_right> Action13 space)? space* <relation_point> Action14)> */
		func() bool {
			position91, tokenIndex91 := position, tokenIndex
			{
				position92 := position
				{
					position93, tokenIndex93 := position, tokenIndex
					{
						position95 := position
						if !_rules[rulecardinality_left]() {
							goto l93
						}
						add(rulePegText, position95)
					}
					if !_rules[ruleAction12]() {
						goto l93
					}
					goto l94
				l93:
					position, tokenIndex = position93, tokenIndex93
				}
			l94:
			l96:
				{
					position97, tokenIndex97 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l97
					}
					goto l96
				l97:
					position, tokenIndex = position97, tokenIndex97
				}
				if buffer[position] != rune('-') {
					goto l91
				}
				position++
				if buffer[position] != rune('-') {
					goto l91
				}
				position++
			l98:
				{
					position99, tokenIndex99 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l99
					}
					goto l98
				l99:
					position, tokenIndex = position99, tokenIndex99
				}
				{
					position100, tokenIndex100 := position, tokenIndex
					{
						position102 := position
						if !_rules[rulecardinality_right]() {
							goto l100
						}
						add(rulePegText, position102)
					}
					if !_rules[ruleAction13]() {
						goto l100
					}
					if !_rules[rulespace]() {
						goto l100
					}
					goto l101
				l100:
					position, tokenIndex = position100, tokenIndex100
				}
			l101:
			l103:
				{
					position104, tokenIndex104 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l104
					}
					goto l103
				l104:
					position, tokenIndex = position104, tokenIndex104
				}
				{
					position105 := position
					if !_rules[rulerelation_point]() {
						goto l91
					}
					add(rulePegText, position105)
				}
				if !_rules[ruleAction14]() {
					goto l91
				}
				add(rulerelation, position92)
			}
			return true
		l91:
			position, tokenIndex = position91, tokenIndex91
			return false
		},
		/* 11 column_comment <- <(space+ '#' space? <comment_string> Action15)> */
		func() bool {
			position106, tokenIndex106 := position, tokenIndex
			{
				position107 := position
				if !_rules[rulespace]() {
					goto l106
				}
			l108:
				{
					position109, tokenIndex109 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l109
					}
					goto l108
				l109:
					position, tokenIndex = position109, tokenIndex109
				}
				if buffer[position] != rune('#') {
					goto l106
				}
				position++
				{
					position110, tokenIndex110 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l110
					}
					goto l111
				l110:
					position, tokenIndex = position110, tokenIndex110
				}
			l111:
				{
					position112 := position
					if !_rules[rulecomment_string]() {
						goto l106
					}
					add(rulePegText, position112)
				}
				if !_rules[ruleAction15]() {
					goto l106
				}
				add(rulecolumn_comment, position107)
			}
			return true
		l106:
			position, tokenIndex = position106, tokenIndex106
			return false
		},
		/* 12 index_info <- <(space+ (('i' / 'I') ('n' / 'N') ('d' / 'D') ('e' / 'E') ('x' / 'X')) space+ <real_column_name> Action16 space+ '(' space* <real_column_name> Action17 (space* ',' space* <real_column_name> Action18 space*)* space* ')' (space+ ('u' 'n' 'i' 'q' 'u' 'e') Action19)? space* newline*)> */
		func() bool {
			position113, tokenIndex113 := position, tokenIndex
			{
				position114 := position
				if !_rules[rulespace]() {
					goto l113
				}
			l115:
				{
					position116, tokenIndex116 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l116
					}
					goto l115
				l116:
					position, tokenIndex = position116, tokenIndex116
				}
				{
					position117, tokenIndex117 := position, tokenIndex
					if buffer[position] != rune('i') {
						goto l118
					}
					position++
					goto l117
				l118:
					position, tokenIndex = position117, tokenIndex117
					if buffer[position] != rune('I') {
						goto l113
					}
					position++
				}
			l117:
				{
					position119, tokenIndex119 := position, tokenIndex
					if buffer[position] != rune('n') {
						goto l120
					}
					position++
					goto l119
				l120:
					position, tokenIndex = position119, tokenIndex119
					if buffer[position] != rune('N') {
						goto l113
					}
					position++
				}
			l119:
				{
					position121, tokenIndex121 := position, tokenIndex
					if buffer[position] != rune('d') {
						goto l122
					}
					position++
					goto l121
				l122:
					position, tokenIndex = position121, tokenIndex121
					if buffer[position] != rune('D') {
						goto l113
					}
					position++
				}
			l121:
				{
					position123, tokenIndex123 := position, tokenIndex
					if buffer[position] != rune('e') {
						goto l124
					}
					position++
					goto l123
				l124:
					position, tokenIndex = position123, tokenIndex123
					if buffer[position] != rune('E') {
						goto l113
					}
					position++
				}
			l123:
				{
					position125, tokenIndex125 := position, tokenIndex
					if buffer[position] != rune('x') {
						goto l126
					}
					position++
					goto l125
				l126:
					position, tokenIndex = position125, tokenIndex125
					if buffer[position] != rune('X') {
						goto l113
					}
					position++
				}
			l125:
				if !_rules[rulespace]() {
					goto l113
				}
			l127:
				{
					position128, tokenIndex128 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l128
					}
					goto l127
				l128:
					position, tokenIndex = position128, tokenIndex128
				}
				{
					position129 := position
					if !_rules[rulereal_column_name]() {
						goto l113
					}
					add(rulePegText, position129)
				}
				if !_rules[ruleAction16]() {
					goto l113
				}
				if !_rules[rulespace]() {
					goto l113
				}
			l130:
				{
					position131, tokenIndex131 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l131
					}
					goto l130
				l131:
					position, tokenIndex = position131, tokenIndex131
				}
				if buffer[position] != rune('(') {
					goto l113
				}
				position++
			l132:
				{
					position133, tokenIndex133 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l133
					}
					goto l132
				l133:
					position, tokenIndex = position133, tokenIndex133
				}
				{
					position134 := position
					if !_rules[rulereal_column_name]() {
						goto l113
					}
					add(rulePegText, position134)
				}
				if !_rules[ruleAction17]() {
					goto l113
				}
			l135:
				{
					position136, tokenIndex136 := position, tokenIndex
				l137:
					{
						position138, tokenIndex138 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l138
						}
						goto l137
					l138:
						position, tokenIndex = position138, tokenIndex138
					}
					if buffer[position] != rune(',') {
						goto l136
					}
					position++
				l139:
					{
						position140, tokenIndex140 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l140
						}
						goto l139
					l140:
						position, tokenIndex = position140, tokenIndex140
					}
					{
						position141 := position
						if !_rules[rulereal_column_name]() {
							goto l136
						}
						add(rulePegText, position141)
					}
					if !_rules[ruleAction18]() {
						goto l136
					}
				l142:
					{
						position143, tokenIndex143 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l143
						}
						goto l142
					l143:
						position, tokenIndex = position143, tokenIndex143
					}
					goto l135
				l136:
					position, tokenIndex = position136, tokenIndex136
				}
			l144:
				{
					position145, tokenIndex145 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l145
					}
					goto l144
				l145:
					position, tokenIndex = position145, tokenIndex145
				}
				if buffer[position] != rune(')') {
					goto l113
				}
				position++
				{
					position146, tokenIndex146 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l146
					}
				l148:
					{
						position149, tokenIndex149 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l149
						}
						goto l148
					l149:
						position, tokenIndex = position149, tokenIndex149
					}
					if buffer[position] != rune('u') {
						goto l146
					}
					position++
					if buffer[position] != rune('n') {
						goto l146
					}
					position++
					if buffer[position] != rune('i') {
						goto l146
					}
					position++
					if buffer[position] != rune('q') {
						goto l146
					}
					position++
					if buffer[position] != rune('u') {
						goto l146
					}
					position++
					if buffer[position] != rune('e') {
						goto l146
					}
					position++
					if !_rules[ruleAction19]() {
						goto l146
					}
					goto l147
				l146:
					position, tokenIndex = position146, tokenIndex146
				}
			l147:
			l150:
				{
					position151, tokenIndex151 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l151
					}
					goto l150
				l151:
					position, tokenIndex = position151, tokenIndex151
				}
			l152:
				{
					position153, tokenIndex153 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l153
					}
					goto l152
				l153:
					position, tokenIndex = position153, tokenIndex153
				}
				add(ruleindex_info, position114)
			}
			return true
		l113:
			position, tokenIndex = position113, tokenIndex113
			return false
		},
		/* 13 title <- <(!('\r' / '\n') .)+> */
		func() bool {
			position154, tokenIndex154 := position, tokenIndex
			{
				position155 := position
				{
					position158, tokenIndex158 := position, tokenIndex
					{
						position159, tokenIndex159 := position, tokenIndex
						if buffer[position] != rune('\r') {
							goto l160
						}
						position++
						goto l159
					l160:
						position, tokenIndex = position159, tokenIndex159
						if buffer[position] != rune('\n') {
							goto l158
						}
						position++
					}
				l159:
					goto l154
				l158:
					position, tokenIndex = position158, tokenIndex158
				}
				if !matchDot() {
					goto l154
				}
			l156:
				{
					position157, tokenIndex157 := position, tokenIndex
					{
						position161, tokenIndex161 := position, tokenIndex
						{
							position162, tokenIndex162 := position, tokenIndex
							if buffer[position] != rune('\r') {
								goto l163
							}
							position++
							goto l162
						l163:
							position, tokenIndex = position162, tokenIndex162
							if buffer[position] != rune('\n') {
								goto l161
							}
							position++
						}
					l162:
						goto l157
					l161:
						position, tokenIndex = position161, tokenIndex161
					}
					if !matchDot() {
						goto l157
					}
					goto l156
				l157:
					position, tokenIndex = position157, tokenIndex157
				}
				add(ruletitle, position155)
			}
			return true
		l154:
			position, tokenIndex = position154, tokenIndex154
			return false
		},
		/* 14 comment_string <- <(!('\r' / '\n') .)*> */
		func() bool {
			{
				position165 := position
			l166:
				{
					position167, tokenIndex167 := position, tokenIndex
					{
						position168, tokenIndex168 := position, tokenIndex
						{
							position169, tokenIndex169 := position, tokenIndex
							if buffer[position] != rune('\r') {
								goto l170
							}
							position++
							goto l169
						l170:
							position, tokenIndex = position169, tokenIndex169
							if buffer[position] != rune('\n') {
								goto l168
							}
							position++
						}
					l169:
						goto l167
					l168:
						position, tokenIndex = position168, tokenIndex168
					}
					if !matchDot() {
						goto l167
					}
					goto l166
				l167:
					position, tokenIndex = position167, tokenIndex167
				}
				add(rulecomment_string, position165)
			}
			return true
		},
		/* 15 whitespace <- <(' ' / '\t' / '\r' / '\n')+> */
		func() bool {
			position171, tokenIndex171 := position, tokenIndex
			{
				position172 := position
				{
					position175, tokenIndex175 := position, tokenIndex
					if buffer[position] != rune(' ') {
						goto l176
					}
					position++
					goto l175
				l176:
					position, tokenIndex = position175, tokenIndex175
					if buffer[position] != rune('\t') {
						goto l177
					}
					position++
					goto l175
				l177:
					position, tokenIndex = position175, tokenIndex175
					if buffer[position] != rune('\r') {
						goto l178
					}
					position++
					goto l175
				l178:
					position, tokenIndex = position175, tokenIndex175
					if buffer[position] != rune('\n') {
						goto l171
					}
					position++
				}
			l175:
			l173:
				{
					position174, tokenIndex174 := position, tokenIndex
					{
						position179, tokenIndex179 := position, tokenIndex
						if buffer[position] != rune(' ') {
							goto l180
						}
						position++
						goto l179
					l180:
						position, tokenIndex = position179, tokenIndex179
						if buffer[position] != rune('\t') {
							goto l181
						}
						position++
						goto l179
					l181:
						position, tokenIndex = position179, tokenIndex179
						if buffer[position] != rune('\r') {
							goto l182
						}
						position++
						goto l179
					l182:
						position, tokenIndex = position179, tokenIndex179
						if buffer[position] != rune('\n') {
							goto l174
						}
						position++
					}
				l179:
					goto l173
				l174:
					position, tokenIndex = position174, tokenIndex174
				}
				add(rulewhitespace, position172)
			}
			return true
		l171:
			position, tokenIndex = position171, tokenIndex171
			return false
		},
		/* 16 newline <- <('\r' / '\n')+> */
		func() bool {
			position183, tokenIndex183 := position, tokenIndex
			{
				position184 := position
				{
					position187, tokenIndex187 := position, tokenIndex
					if buffer[position] != rune('\r') {
						goto l188
					}
					position++
					goto l187
				l188:
					position, tokenIndex = position187, tokenIndex187
					if buffer[position] != rune('\n') {
						goto l183
					}
					position++
				}
			l187:
			l185:
				{
					position186, tokenIndex186 := position, tokenIndex
					{
						position189, tokenIndex189 := position, tokenIndex
						if buffer[position] != rune('\r') {
							goto l190
						}
						position++
						goto l189
					l190:
						position, tokenIndex = position189, tokenIndex189
						if buffer[position] != rune('\n') {
							goto l186
						}
						position++
					}
				l189:
					goto l185
				l186:
					position, tokenIndex = position186, tokenIndex186
				}
				add(rulenewline, position184)
			}
			return true
		l183:
			position, tokenIndex = position183, tokenIndex183
			return false
		},
		/* 17 space <- <(' ' / '\t')+> */
		func() bool {
			position191, tokenIndex191 := position, tokenIndex
			{
				position192 := position
				{
					position195, tokenIndex195 := position, tokenIndex
					if buffer[position] != rune(' ') {
						goto l196
					}
					position++
					goto l195
				l196:
					position, tokenIndex = position195, tokenIndex195
					if buffer[position] != rune('\t') {
						goto l191
					}
					position++
				}
			l195:
			l193:
				{
					position194, tokenIndex194 := position, tokenIndex
					{
						position197, tokenIndex197 := position, tokenIndex
						if buffer[position] != rune(' ') {
							goto l198
						}
						position++
						goto l197
					l198:
						position, tokenIndex = position197, tokenIndex197
						if buffer[position] != rune('\t') {
							goto l194
						}
						position++
					}
				l197:
					goto l193
				l194:
					position, tokenIndex = position194, tokenIndex194
				}
				add(rulespace, position192)
			}
			return true
		l191:
			position, tokenIndex = position191, tokenIndex191
			return false
		},
		/* 18 notnull <- <('N' 'N')> */
		func() bool {
			position199, tokenIndex199 := position, tokenIndex
			{
				position200 := position
				if buffer[position] != rune('N') {
					goto l199
				}
				position++
				if buffer[position] != rune('N') {
					goto l199
				}
				position++
				add(rulenotnull, position200)
			}
			return true
		l199:
			position, tokenIndex = position199, tokenIndex199
			return false
		},
		/* 19 unique <- <'U'> */
		func() bool {
			position201, tokenIndex201 := position, tokenIndex
			{
				position202 := position
				if buffer[position] != rune('U') {
					goto l201
				}
				position++
				add(ruleunique, position202)
			}
			return true
		l201:
			position, tokenIndex = position201, tokenIndex201
			return false
		},
		/* 20 real_table_name <- <([a-z] / [A-Z] / [0-9] / '_')+> */
		func() bool {
			position203, tokenIndex203 := position, tokenIndex
			{
				position204 := position
				{
					position207, tokenIndex207 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l208
					}
					position++
					goto l207
				l208:
					position, tokenIndex = position207, tokenIndex207
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l209
					}
					position++
					goto l207
				l209:
					position, tokenIndex = position207, tokenIndex207
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l210
					}
					position++
					goto l207
				l210:
					position, tokenIndex = position207, tokenIndex207
					if buffer[position] != rune('_') {
						goto l203
					}
					position++
				}
			l207:
			l205:
				{
					position206, tokenIndex206 := position, tokenIndex
					{
						position211, tokenIndex211 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l212
						}
						position++
						goto l211
					l212:
						position, tokenIndex = position211, tokenIndex211
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l213
						}
						position++
						goto l211
					l213:
						position, tokenIndex = position211, tokenIndex211
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l214
						}
						position++
						goto l211
					l214:
						position, tokenIndex = position211, tokenIndex211
						if buffer[position] != rune('_') {
							goto l206
						}
						position++
					}
				l211:
					goto l205
				l206:
					position, tokenIndex = position206, tokenIndex206
				}
				add(rulereal_table_name, position204)
			}
			return true
		l203:
			position, tokenIndex = position203, tokenIndex203
			return false
		},
		/* 21 table_name <- <(!whitespace .)*> */
		func() bool {
			{
				position216 := position
			l217:
				{
					position218, tokenIndex218 := position, tokenIndex
					{
						position219, tokenIndex219 := position, tokenIndex
						if !_rules[rulewhitespace]() {
							goto l219
						}
						goto l218
					l219:
						position, tokenIndex = position219, tokenIndex219
					}
					if !matchDot() {
						goto l218
					}
					goto l217
				l218:
					position, tokenIndex = position218, tokenIndex218
				}
				add(ruletable_name, position216)
			}
			return true
		},
		/* 22 real_column_name <- <([a-z] / [A-Z] / [0-9] / '_')+> */
		func() bool {
			position220, tokenIndex220 := position, tokenIndex
			{
				position221 := position
				{
					position224, tokenIndex224 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l225
					}
					position++
					goto l224
				l225:
					position, tokenIndex = position224, tokenIndex224
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l226
					}
					position++
					goto l224
				l226:
					position, tokenIndex = position224, tokenIndex224
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l227
					}
					position++
					goto l224
				l227:
					position, tokenIndex = position224, tokenIndex224
					if buffer[position] != rune('_') {
						goto l220
					}
					position++
				}
			l224:
			l222:
				{
					position223, tokenIndex223 := position, tokenIndex
					{
						position228, tokenIndex228 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l229
						}
						position++
						goto l228
					l229:
						position, tokenIndex = position228, tokenIndex228
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l230
						}
						position++
						goto l228
					l230:
						position, tokenIndex = position228, tokenIndex228
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l231
						}
						position++
						goto l228
					l231:
						position, tokenIndex = position228, tokenIndex228
						if buffer[position] != rune('_') {
							goto l223
						}
						position++
					}
				l228:
					goto l222
				l223:
					position, tokenIndex = position223, tokenIndex223
				}
				add(rulereal_column_name, position221)
			}
			return true
		l220:
			position, tokenIndex = position220, tokenIndex220
			return false
		},
		/* 23 column_name <- <(!(' ' / '\t' / '\r' / '\n') .)+> */
		func() bool {
			position232, tokenIndex232 := position, tokenIndex
			{
				position233 := position
				{
					position236, tokenIndex236 := position, tokenIndex
					{
						position237, tokenIndex237 := position, tokenIndex
						if buffer[position] != rune(' ') {
							goto l238
						}
						position++
						goto l237
					l238:
						position, tokenIndex = position237, tokenIndex237
						if buffer[position] != rune('\t') {
							goto l239
						}
						position++
						goto l237
					l239:
						position, tokenIndex = position237, tokenIndex237
						if buffer[position] != rune('\r') {
							goto l240
						}
						position++
						goto l237
					l240:
						position, tokenIndex = position237, tokenIndex237
						if buffer[position] != rune('\n') {
							goto l236
						}
						position++
					}
				l237:
					goto l232
				l236:
					position, tokenIndex = position236, tokenIndex236
				}
				if !matchDot() {
					goto l232
				}
			l234:
				{
					position235, tokenIndex235 := position, tokenIndex
					{
						position241, tokenIndex241 := position, tokenIndex
						{
							position242, tokenIndex242 := position, tokenIndex
							if buffer[position] != rune(' ') {
								goto l243
							}
							position++
							goto l242
						l243:
							position, tokenIndex = position242, tokenIndex242
							if buffer[position] != rune('\t') {
								goto l244
							}
							position++
							goto l242
						l244:
							position, tokenIndex = position242, tokenIndex242
							if buffer[position] != rune('\r') {
								goto l245
							}
							position++
							goto l242
						l245:
							position, tokenIndex = position242, tokenIndex242
							if buffer[position] != rune('\n') {
								goto l241
							}
							position++
						}
					l242:
						goto l235
					l241:
						position, tokenIndex = position241, tokenIndex241
					}
					if !matchDot() {
						goto l235
					}
					goto l234
				l235:
					position, tokenIndex = position235, tokenIndex235
				}
				add(rulecolumn_name, position233)
			}
			return true
		l232:
			position, tokenIndex = position232, tokenIndex232
			return false
		},
		/* 24 relation_point <- <([a-z] / [A-Z] / [0-9] / '_' / '.')+> */
		func() bool {
			position246, tokenIndex246 := position, tokenIndex
			{
				position247 := position
				{
					position250, tokenIndex250 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l251
					}
					position++
					goto l250
				l251:
					position, tokenIndex = position250, tokenIndex250
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l252
					}
					position++
					goto l250
				l252:
					position, tokenIndex = position250, tokenIndex250
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l253
					}
					position++
					goto l250
				l253:
					position, tokenIndex = position250, tokenIndex250
					if buffer[position] != rune('_') {
						goto l254
					}
					position++
					goto l250
				l254:
					position, tokenIndex = position250, tokenIndex250
					if buffer[position] != rune('.') {
						goto l246
					}
					position++
				}
			l250:
			l248:
				{
					position249, tokenIndex249 := position, tokenIndex
					{
						position255, tokenIndex255 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l256
						}
						position++
						goto l255
					l256:
						position, tokenIndex = position255, tokenIndex255
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l257
						}
						position++
						goto l255
					l257:
						position, tokenIndex = position255, tokenIndex255
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l258
						}
						position++
						goto l255
					l258:
						position, tokenIndex = position255, tokenIndex255
						if buffer[position] != rune('_') {
							goto l259
						}
						position++
						goto l255
					l259:
						position, tokenIndex = position255, tokenIndex255
						if buffer[position] != rune('.') {
							goto l249
						}
						position++
					}
				l255:
					goto l248
				l249:
					position, tokenIndex = position249, tokenIndex249
				}
				add(rulerelation_point, position247)
			}
			return true
		l246:
			position, tokenIndex = position246, tokenIndex246
			return false
		},
		/* 25 pkey <- <('+' / '*')> */
		func() bool {
			position260, tokenIndex260 := position, tokenIndex
			{
				position261 := position
				{
					position262, tokenIndex262 := position, tokenIndex
					if buffer[position] != rune('+') {
						goto l263
					}
					position++
					goto l262
				l263:
					position, tokenIndex = position262, tokenIndex262
					if buffer[position] != rune('*') {
						goto l260
					}
					position++
				}
			l262:
				add(rulepkey, position261)
			}
			return true
		l260:
			position, tokenIndex = position260, tokenIndex260
			return false
		},
		/* 26 col_type <- <([a-z] / [A-Z] / [0-9] / '_' / '(' / ')' / ' ' / '.' / ',')+> */
		func() bool {
			position264, tokenIndex264 := position, tokenIndex
			{
				position265 := position
				{
					position268, tokenIndex268 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l269
					}
					position++
					goto l268
				l269:
					position, tokenIndex = position268, tokenIndex268
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l270
					}
					position++
					goto l268
				l270:
					position, tokenIndex = position268, tokenIndex268
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l271
					}
					position++
					goto l268
				l271:
					position, tokenIndex = position268, tokenIndex268
					if buffer[position] != rune('_') {
						goto l272
					}
					position++
					goto l268
				l272:
					position, tokenIndex = position268, tokenIndex268
					if buffer[position] != rune('(') {
						goto l273
					}
					position++
					goto l268
				l273:
					position, tokenIndex = position268, tokenIndex268
					if buffer[position] != rune(')') {
						goto l274
					}
					position++
					goto l268
				l274:
					position, tokenIndex = position268, tokenIndex268
					if buffer[position] != rune(' ') {
						goto l275
					}
					position++
					goto l268
				l275:
					position, tokenIndex = position268, tokenIndex268
					if buffer[position] != rune('.') {
						goto l276
					}
					position++
					goto l268
				l276:
					position, tokenIndex = position268, tokenIndex268
					if buffer[position] != rune(',') {
						goto l264
					}
					position++
				}
			l268:
			l266:
				{
					position267, tokenIndex267 := position, tokenIndex
					{
						position277, tokenIndex277 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l278
						}
						position++
						goto l277
					l278:
						position, tokenIndex = position277, tokenIndex277
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l279
						}
						position++
						goto l277
					l279:
						position, tokenIndex = position277, tokenIndex277
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l280
						}
						position++
						goto l277
					l280:
						position, tokenIndex = position277, tokenIndex277
						if buffer[position] != rune('_') {
							goto l281
						}
						position++
						goto l277
					l281:
						position, tokenIndex = position277, tokenIndex277
						if buffer[position] != rune('(') {
							goto l282
						}
						position++
						goto l277
					l282:
						position, tokenIndex = position277, tokenIndex277
						if buffer[position] != rune(')') {
							goto l283
						}
						position++
						goto l277
					l283:
						position, tokenIndex = position277, tokenIndex277
						if buffer[position] != rune(' ') {
							goto l284
						}
						position++
						goto l277
					l284:
						position, tokenIndex = position277, tokenIndex277
						if buffer[position] != rune('.') {
							goto l285
						}
						position++
						goto l277
					l285:
						position, tokenIndex = position277, tokenIndex277
						if buffer[position] != rune(',') {
							goto l267
						}
						position++
					}
				l277:
					goto l266
				l267:
					position, tokenIndex = position267, tokenIndex267
				}
				add(rulecol_type, position265)
			}
			return true
		l264:
			position, tokenIndex = position264, tokenIndex264
			return false
		},
		/* 27 default <- <((!('\r' / '\n' / ']') .) / ('\\' ']'))*> */
		func() bool {
			{
				position287 := position
			l288:
				{
					position289, tokenIndex289 := position, tokenIndex
					{
						position290, tokenIndex290 := position, tokenIndex
						{
							position292, tokenIndex292 := position, tokenIndex
							{
								position293, tokenIndex293 := position, tokenIndex
								if buffer[position] != rune('\r') {
									goto l294
								}
								position++
								goto l293
							l294:
								position, tokenIndex = position293, tokenIndex293
								if buffer[position] != rune('\n') {
									goto l295
								}
								position++
								goto l293
							l295:
								position, tokenIndex = position293, tokenIndex293
								if buffer[position] != rune(']') {
									goto l292
								}
								position++
							}
						l293:
							goto l291
						l292:
							position, tokenIndex = position292, tokenIndex292
						}
						if !matchDot() {
							goto l291
						}
						goto l290
					l291:
						position, tokenIndex = position290, tokenIndex290
						if buffer[position] != rune('\\') {
							goto l289
						}
						position++
						if buffer[position] != rune(']') {
							goto l289
						}
						position++
					}
				l290:
					goto l288
				l289:
					position, tokenIndex = position289, tokenIndex289
				}
				add(ruledefault, position287)
			}
			return true
		},
		/* 28 cardinality_right <- <cardinality> */
		func() bool {
			position296, tokenIndex296 := position, tokenIndex
			{
				position297 := position
				if !_rules[rulecardinality]() {
					goto l296
				}
				add(rulecardinality_right, position297)
			}
			return true
		l296:
			position, tokenIndex = position296, tokenIndex296
			return false
		},
		/* 29 cardinality_left <- <cardinality> */
		func() bool {
			position298, tokenIndex298 := position, tokenIndex
			{
				position299 := position
				if !_rules[rulecardinality]() {
					goto l298
				}
				add(rulecardinality_left, position299)
			}
			return true
		l298:
			position, tokenIndex = position298, tokenIndex298
			return false
		},
		/* 30 cardinality <- <(('0' / '1' / '*') (. . ('0' / '1' / '*'))?)> */
		func() bool {
			position300, tokenIndex300 := position, tokenIndex
			{
				position301 := position
				{
					position302, tokenIndex302 := position, tokenIndex
					if buffer[position] != rune('0') {
						goto l303
					}
					position++
					goto l302
				l303:
					position, tokenIndex = position302, tokenIndex302
					if buffer[position] != rune('1') {
						goto l304
					}
					position++
					goto l302
				l304:
					position, tokenIndex = position302, tokenIndex302
					if buffer[position] != rune('*') {
						goto l300
					}
					position++
				}
			l302:
				{
					position305, tokenIndex305 := position, tokenIndex
					if !matchDot() {
						goto l305
					}
					if !matchDot() {
						goto l305
					}
					{
						position307, tokenIndex307 := position, tokenIndex
						if buffer[position] != rune('0') {
							goto l308
						}
						position++
						goto l307
					l308:
						position, tokenIndex = position307, tokenIndex307
						if buffer[position] != rune('1') {
							goto l309
						}
						position++
						goto l307
					l309:
						position, tokenIndex = position307, tokenIndex307
						if buffer[position] != rune('*') {
							goto l305
						}
						position++
					}
				l307:
					goto l306
				l305:
					position, tokenIndex = position305, tokenIndex305
				}
			l306:
				add(rulecardinality, position301)
			}
			return true
		l300:
			position, tokenIndex = position300, tokenIndex300
			return false
		},
		nil,
		/* 33 Action0 <- <{p.Err(begin, buffer)}> */
		func() bool {
			{
				add(ruleAction0, position)
			}
			return true
		},
		/* 34 Action1 <- <{p.Err(begin, buffer)}> */
		func() bool {
			{
				add(ruleAction1, position)
			}
			return true
		},
		/* 35 Action2 <- <{p.setTitle(text)}> */
		func() bool {
			{
				add(ruleAction2, position)
			}
			return true
		},
		/* 36 Action3 <- <{p.addTableTitleReal(text)}> */
		func() bool {
			{
				add(ruleAction3, position)
			}
			return true
		},
		/* 37 Action4 <- <{p.addTableTitle(text)}> */
		func() bool {
			{
				add(ruleAction4, position)
			}
			return true
		},
		/* 38 Action5 <- <{ p.addPrimaryKey(text) }> */
		func() bool {
			{
				add(ruleAction5, position)
			}
			return true
		},
		/* 39 Action6 <- <{ p.setColumnNameReal(text) }> */
		func() bool {
			{
				add(ruleAction6, position)
			}
			return true
		},
		/* 40 Action7 <- <{ p.setColumnName(text) }> */
		func() bool {
			{
				add(ruleAction7, position)
			}
			return true
		},
		/* 41 Action8 <- <{ p.addColumnType(text) }> */
		func() bool {
			{
				add(ruleAction8, position)
			}
			return true
		},
		/* 42 Action9 <- <{ p.setNotNull() }> */
		func() bool {
			{
				add(ruleAction9, position)
			}
			return true
		},
		/* 43 Action10 <- <{ p.setUnique() }> */
		func() bool {
			{
				add(ruleAction10, position)
			}
			return true
		},
		/* 44 Action11 <- <{ p.setColumnDefault(text) }> */
		func() bool {
			{
				add(ruleAction11, position)
			}
			return true
		},
		/* 45 Action12 <- <{ p.setRelationSource(text) }> */
		func() bool {
			{
				add(ruleAction12, position)
			}
			return true
		},
		/* 46 Action13 <- <{ p.setRelationDestination(text) }> */
		func() bool {
			{
				add(ruleAction13, position)
			}
			return true
		},
		/* 47 Action14 <- <{ p.setRelationTableNameReal(text) }> */
		func() bool {
			{
				add(ruleAction14, position)
			}
			return true
		},
		/* 48 Action15 <- <{ p.addComment(text) }> */
		func() bool {
			{
				add(ruleAction15, position)
			}
			return true
		},
		/* 49 Action16 <- <{p.setIndexName(text)}> */
		func() bool {
			{
				add(ruleAction16, position)
			}
			return true
		},
		/* 50 Action17 <- <{p.setIndexColumn(text)}> */
		func() bool {
			{
				add(ruleAction17, position)
			}
			return true
		},
		/* 51 Action18 <- <{p.setIndexColumn(text)}> */
		func() bool {
			{
				add(ruleAction18, position)
			}
			return true
		},
		/* 52 Action19 <- <{ p.setUniqueIndex() }> */
		func() bool {
			{
				add(ruleAction19, position)
			}
			return true
		},
	}
	p.rules = _rules
}
