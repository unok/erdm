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
)

var rul3s = [...]string{
	"Unknown",
	"root",
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
	rules  [50]func() bool
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
			p.setTitle(text)
		case ruleAction1:
			p.addTableTitleReal(text)
		case ruleAction2:
			p.addTableTitle(text)
		case ruleAction3:
			p.addPrimaryKey(text)
		case ruleAction4:
			p.setColumnNameReal(text)
		case ruleAction5:
			p.setColumnName(text)
		case ruleAction6:
			p.addColumnType(text)
		case ruleAction7:
			p.setNotNull()
		case ruleAction8:
			p.setUnique()
		case ruleAction9:
			p.setColumnDefault(text)
		case ruleAction10:
			p.setRelationSource(text)
		case ruleAction11:
			p.setRelationDestination(text)
		case ruleAction12:
			p.setRelationTableNameReal(text)
		case ruleAction13:
			p.addComment(text)
		case ruleAction14:
			p.setIndexName(text)
		case ruleAction15:
			p.setIndexColumn(text)
		case ruleAction16:
			p.setIndexColumn(text)
		case ruleAction17:
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
		/* 0 root <- <expression> */
		func() bool {
			position0, tokenIndex0 := position, tokenIndex
			{
				position1 := position
				if !_rules[ruleexpression]() {
					goto l0
				}
				add(ruleroot, position1)
			}
			return true
		l0:
			position, tokenIndex = position0, tokenIndex0
			return false
		},
		/* 1 expression <- <(title_info (table_info / comment / empty_line)*)> */
		func() bool {
			position2, tokenIndex2 := position, tokenIndex
			{
				position3 := position
				if !_rules[ruletitle_info]() {
					goto l2
				}
			l4:
				{
					position5, tokenIndex5 := position, tokenIndex
					{
						position6, tokenIndex6 := position, tokenIndex
						if !_rules[ruletable_info]() {
							goto l7
						}
						goto l6
					l7:
						position, tokenIndex = position6, tokenIndex6
						if !_rules[rulecomment]() {
							goto l8
						}
						goto l6
					l8:
						position, tokenIndex = position6, tokenIndex6
						if !_rules[ruleempty_line]() {
							goto l5
						}
					}
				l6:
					goto l4
				l5:
					position, tokenIndex = position5, tokenIndex5
				}
				add(ruleexpression, position3)
			}
			return true
		l2:
			position, tokenIndex = position2, tokenIndex2
			return false
		},
		/* 2 title_info <- <('#' space* ('T' 'i' 't' 'l' 'e' ':') space* <title> Action0 newline)> */
		func() bool {
			position9, tokenIndex9 := position, tokenIndex
			{
				position10 := position
				if buffer[position] != rune('#') {
					goto l9
				}
				position++
			l11:
				{
					position12, tokenIndex12 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l12
					}
					goto l11
				l12:
					position, tokenIndex = position12, tokenIndex12
				}
				if buffer[position] != rune('T') {
					goto l9
				}
				position++
				if buffer[position] != rune('i') {
					goto l9
				}
				position++
				if buffer[position] != rune('t') {
					goto l9
				}
				position++
				if buffer[position] != rune('l') {
					goto l9
				}
				position++
				if buffer[position] != rune('e') {
					goto l9
				}
				position++
				if buffer[position] != rune(':') {
					goto l9
				}
				position++
			l13:
				{
					position14, tokenIndex14 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l14
					}
					goto l13
				l14:
					position, tokenIndex = position14, tokenIndex14
				}
				{
					position15 := position
					if !_rules[ruletitle]() {
						goto l9
					}
					add(rulePegText, position15)
				}
				if !_rules[ruleAction0]() {
					goto l9
				}
				if !_rules[rulenewline]() {
					goto l9
				}
				add(ruletitle_info, position10)
			}
			return true
		l9:
			position, tokenIndex = position9, tokenIndex9
			return false
		},
		/* 3 table_info <- <(table_name_info column_info* index_info*)> */
		func() bool {
			position16, tokenIndex16 := position, tokenIndex
			{
				position17 := position
				if !_rules[ruletable_name_info]() {
					goto l16
				}
			l18:
				{
					position19, tokenIndex19 := position, tokenIndex
					if !_rules[rulecolumn_info]() {
						goto l19
					}
					goto l18
				l19:
					position, tokenIndex = position19, tokenIndex19
				}
			l20:
				{
					position21, tokenIndex21 := position, tokenIndex
					if !_rules[ruleindex_info]() {
						goto l21
					}
					goto l20
				l21:
					position, tokenIndex = position21, tokenIndex21
				}
				add(ruletable_info, position17)
			}
			return true
		l16:
			position, tokenIndex = position16, tokenIndex16
			return false
		},
		/* 4 comment <- <(space* ('/' '/') comment_string newline)> */
		func() bool {
			position22, tokenIndex22 := position, tokenIndex
			{
				position23 := position
			l24:
				{
					position25, tokenIndex25 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l25
					}
					goto l24
				l25:
					position, tokenIndex = position25, tokenIndex25
				}
				if buffer[position] != rune('/') {
					goto l22
				}
				position++
				if buffer[position] != rune('/') {
					goto l22
				}
				position++
				if !_rules[rulecomment_string]() {
					goto l22
				}
				if !_rules[rulenewline]() {
					goto l22
				}
				add(rulecomment, position23)
			}
			return true
		l22:
			position, tokenIndex = position22, tokenIndex22
			return false
		},
		/* 5 empty_line <- <whitespace> */
		func() bool {
			position26, tokenIndex26 := position, tokenIndex
			{
				position27 := position
				if !_rules[rulewhitespace]() {
					goto l26
				}
				add(ruleempty_line, position27)
			}
			return true
		l26:
			position, tokenIndex = position26, tokenIndex26
			return false
		},
		/* 6 table_name_info <- <(<real_table_name> Action1 space* ('/' space* <table_name> Action2) space* newline*)> */
		func() bool {
			position28, tokenIndex28 := position, tokenIndex
			{
				position29 := position
				{
					position30 := position
					if !_rules[rulereal_table_name]() {
						goto l28
					}
					add(rulePegText, position30)
				}
				if !_rules[ruleAction1]() {
					goto l28
				}
			l31:
				{
					position32, tokenIndex32 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l32
					}
					goto l31
				l32:
					position, tokenIndex = position32, tokenIndex32
				}
				if buffer[position] != rune('/') {
					goto l28
				}
				position++
			l33:
				{
					position34, tokenIndex34 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l34
					}
					goto l33
				l34:
					position, tokenIndex = position34, tokenIndex34
				}
				{
					position35 := position
					if !_rules[ruletable_name]() {
						goto l28
					}
					add(rulePegText, position35)
				}
				if !_rules[ruleAction2]() {
					goto l28
				}
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
			l38:
				{
					position39, tokenIndex39 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l39
					}
					goto l38
				l39:
					position, tokenIndex = position39, tokenIndex39
				}
				add(ruletable_name_info, position29)
			}
			return true
		l28:
			position, tokenIndex = position28, tokenIndex28
			return false
		},
		/* 7 column_info <- <(column_attribute (space* relation (space* relation)*)? (newline? column_comment)* newline?)> */
		func() bool {
			position40, tokenIndex40 := position, tokenIndex
			{
				position41 := position
				if !_rules[rulecolumn_attribute]() {
					goto l40
				}
				{
					position42, tokenIndex42 := position, tokenIndex
				l44:
					{
						position45, tokenIndex45 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l45
						}
						goto l44
					l45:
						position, tokenIndex = position45, tokenIndex45
					}
					if !_rules[rulerelation]() {
						goto l42
					}
				l46:
					{
						position47, tokenIndex47 := position, tokenIndex
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
						if !_rules[rulerelation]() {
							goto l47
						}
						goto l46
					l47:
						position, tokenIndex = position47, tokenIndex47
					}
					goto l43
				l42:
					position, tokenIndex = position42, tokenIndex42
				}
			l43:
			l50:
				{
					position51, tokenIndex51 := position, tokenIndex
					{
						position52, tokenIndex52 := position, tokenIndex
						if !_rules[rulenewline]() {
							goto l52
						}
						goto l53
					l52:
						position, tokenIndex = position52, tokenIndex52
					}
				l53:
					if !_rules[rulecolumn_comment]() {
						goto l51
					}
					goto l50
				l51:
					position, tokenIndex = position51, tokenIndex51
				}
				{
					position54, tokenIndex54 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l54
					}
					goto l55
				l54:
					position, tokenIndex = position54, tokenIndex54
				}
			l55:
				add(rulecolumn_info, position41)
			}
			return true
		l40:
			position, tokenIndex = position40, tokenIndex40
			return false
		},
		/* 8 column_attribute <- <(space+ (<pkey> Action3)? <real_column_name> Action4 ('/' <column_name> Action5)? space+ '[' <col_type> Action6 ']' (('[' notnull Action7 ']') / ('[' unique Action8 ']'))* ('[' '=' <default> Action9 ']')? newline?)> */
		func() bool {
			position56, tokenIndex56 := position, tokenIndex
			{
				position57 := position
				if !_rules[rulespace]() {
					goto l56
				}
			l58:
				{
					position59, tokenIndex59 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l59
					}
					goto l58
				l59:
					position, tokenIndex = position59, tokenIndex59
				}
				{
					position60, tokenIndex60 := position, tokenIndex
					{
						position62 := position
						if !_rules[rulepkey]() {
							goto l60
						}
						add(rulePegText, position62)
					}
					if !_rules[ruleAction3]() {
						goto l60
					}
					goto l61
				l60:
					position, tokenIndex = position60, tokenIndex60
				}
			l61:
				{
					position63 := position
					if !_rules[rulereal_column_name]() {
						goto l56
					}
					add(rulePegText, position63)
				}
				if !_rules[ruleAction4]() {
					goto l56
				}
				{
					position64, tokenIndex64 := position, tokenIndex
					if buffer[position] != rune('/') {
						goto l64
					}
					position++
					{
						position66 := position
						if !_rules[rulecolumn_name]() {
							goto l64
						}
						add(rulePegText, position66)
					}
					if !_rules[ruleAction5]() {
						goto l64
					}
					goto l65
				l64:
					position, tokenIndex = position64, tokenIndex64
				}
			l65:
				if !_rules[rulespace]() {
					goto l56
				}
			l67:
				{
					position68, tokenIndex68 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l68
					}
					goto l67
				l68:
					position, tokenIndex = position68, tokenIndex68
				}
				if buffer[position] != rune('[') {
					goto l56
				}
				position++
				{
					position69 := position
					if !_rules[rulecol_type]() {
						goto l56
					}
					add(rulePegText, position69)
				}
				if !_rules[ruleAction6]() {
					goto l56
				}
				if buffer[position] != rune(']') {
					goto l56
				}
				position++
			l70:
				{
					position71, tokenIndex71 := position, tokenIndex
					{
						position72, tokenIndex72 := position, tokenIndex
						if buffer[position] != rune('[') {
							goto l73
						}
						position++
						if !_rules[rulenotnull]() {
							goto l73
						}
						if !_rules[ruleAction7]() {
							goto l73
						}
						if buffer[position] != rune(']') {
							goto l73
						}
						position++
						goto l72
					l73:
						position, tokenIndex = position72, tokenIndex72
						if buffer[position] != rune('[') {
							goto l71
						}
						position++
						if !_rules[ruleunique]() {
							goto l71
						}
						if !_rules[ruleAction8]() {
							goto l71
						}
						if buffer[position] != rune(']') {
							goto l71
						}
						position++
					}
				l72:
					goto l70
				l71:
					position, tokenIndex = position71, tokenIndex71
				}
				{
					position74, tokenIndex74 := position, tokenIndex
					if buffer[position] != rune('[') {
						goto l74
					}
					position++
					if buffer[position] != rune('=') {
						goto l74
					}
					position++
					{
						position76 := position
						if !_rules[ruledefault]() {
							goto l74
						}
						add(rulePegText, position76)
					}
					if !_rules[ruleAction9]() {
						goto l74
					}
					if buffer[position] != rune(']') {
						goto l74
					}
					position++
					goto l75
				l74:
					position, tokenIndex = position74, tokenIndex74
				}
			l75:
				{
					position77, tokenIndex77 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l77
					}
					goto l78
				l77:
					position, tokenIndex = position77, tokenIndex77
				}
			l78:
				add(rulecolumn_attribute, position57)
			}
			return true
		l56:
			position, tokenIndex = position56, tokenIndex56
			return false
		},
		/* 9 relation <- <(<cardinality_left> Action10 space* ('-' '-') space* <cardinality_right> Action11 space+ <relation_point> Action12)> */
		func() bool {
			position79, tokenIndex79 := position, tokenIndex
			{
				position80 := position
				{
					position81 := position
					if !_rules[rulecardinality_left]() {
						goto l79
					}
					add(rulePegText, position81)
				}
				if !_rules[ruleAction10]() {
					goto l79
				}
			l82:
				{
					position83, tokenIndex83 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l83
					}
					goto l82
				l83:
					position, tokenIndex = position83, tokenIndex83
				}
				if buffer[position] != rune('-') {
					goto l79
				}
				position++
				if buffer[position] != rune('-') {
					goto l79
				}
				position++
			l84:
				{
					position85, tokenIndex85 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l85
					}
					goto l84
				l85:
					position, tokenIndex = position85, tokenIndex85
				}
				{
					position86 := position
					if !_rules[rulecardinality_right]() {
						goto l79
					}
					add(rulePegText, position86)
				}
				if !_rules[ruleAction11]() {
					goto l79
				}
				if !_rules[rulespace]() {
					goto l79
				}
			l87:
				{
					position88, tokenIndex88 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l88
					}
					goto l87
				l88:
					position, tokenIndex = position88, tokenIndex88
				}
				{
					position89 := position
					if !_rules[rulerelation_point]() {
						goto l79
					}
					add(rulePegText, position89)
				}
				if !_rules[ruleAction12]() {
					goto l79
				}
				add(rulerelation, position80)
			}
			return true
		l79:
			position, tokenIndex = position79, tokenIndex79
			return false
		},
		/* 10 column_comment <- <(space+ '#' space? <comment_string> Action13)> */
		func() bool {
			position90, tokenIndex90 := position, tokenIndex
			{
				position91 := position
				if !_rules[rulespace]() {
					goto l90
				}
			l92:
				{
					position93, tokenIndex93 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l93
					}
					goto l92
				l93:
					position, tokenIndex = position93, tokenIndex93
				}
				if buffer[position] != rune('#') {
					goto l90
				}
				position++
				{
					position94, tokenIndex94 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l94
					}
					goto l95
				l94:
					position, tokenIndex = position94, tokenIndex94
				}
			l95:
				{
					position96 := position
					if !_rules[rulecomment_string]() {
						goto l90
					}
					add(rulePegText, position96)
				}
				if !_rules[ruleAction13]() {
					goto l90
				}
				add(rulecolumn_comment, position91)
			}
			return true
		l90:
			position, tokenIndex = position90, tokenIndex90
			return false
		},
		/* 11 index_info <- <(space+ (('i' / 'I') ('n' / 'N') ('d' / 'D') ('e' / 'E') ('x' / 'X')) space+ <real_column_name> Action14 space+ '(' space* <real_column_name> Action15 (space* ',' space* <real_column_name> Action16 space*)* space* ')' (space+ ('u' 'n' 'i' 'q' 'u' 'e') Action17)? space* newline*)> */
		func() bool {
			position97, tokenIndex97 := position, tokenIndex
			{
				position98 := position
				if !_rules[rulespace]() {
					goto l97
				}
			l99:
				{
					position100, tokenIndex100 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l100
					}
					goto l99
				l100:
					position, tokenIndex = position100, tokenIndex100
				}
				{
					position101, tokenIndex101 := position, tokenIndex
					if buffer[position] != rune('i') {
						goto l102
					}
					position++
					goto l101
				l102:
					position, tokenIndex = position101, tokenIndex101
					if buffer[position] != rune('I') {
						goto l97
					}
					position++
				}
			l101:
				{
					position103, tokenIndex103 := position, tokenIndex
					if buffer[position] != rune('n') {
						goto l104
					}
					position++
					goto l103
				l104:
					position, tokenIndex = position103, tokenIndex103
					if buffer[position] != rune('N') {
						goto l97
					}
					position++
				}
			l103:
				{
					position105, tokenIndex105 := position, tokenIndex
					if buffer[position] != rune('d') {
						goto l106
					}
					position++
					goto l105
				l106:
					position, tokenIndex = position105, tokenIndex105
					if buffer[position] != rune('D') {
						goto l97
					}
					position++
				}
			l105:
				{
					position107, tokenIndex107 := position, tokenIndex
					if buffer[position] != rune('e') {
						goto l108
					}
					position++
					goto l107
				l108:
					position, tokenIndex = position107, tokenIndex107
					if buffer[position] != rune('E') {
						goto l97
					}
					position++
				}
			l107:
				{
					position109, tokenIndex109 := position, tokenIndex
					if buffer[position] != rune('x') {
						goto l110
					}
					position++
					goto l109
				l110:
					position, tokenIndex = position109, tokenIndex109
					if buffer[position] != rune('X') {
						goto l97
					}
					position++
				}
			l109:
				if !_rules[rulespace]() {
					goto l97
				}
			l111:
				{
					position112, tokenIndex112 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l112
					}
					goto l111
				l112:
					position, tokenIndex = position112, tokenIndex112
				}
				{
					position113 := position
					if !_rules[rulereal_column_name]() {
						goto l97
					}
					add(rulePegText, position113)
				}
				if !_rules[ruleAction14]() {
					goto l97
				}
				if !_rules[rulespace]() {
					goto l97
				}
			l114:
				{
					position115, tokenIndex115 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l115
					}
					goto l114
				l115:
					position, tokenIndex = position115, tokenIndex115
				}
				if buffer[position] != rune('(') {
					goto l97
				}
				position++
			l116:
				{
					position117, tokenIndex117 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l117
					}
					goto l116
				l117:
					position, tokenIndex = position117, tokenIndex117
				}
				{
					position118 := position
					if !_rules[rulereal_column_name]() {
						goto l97
					}
					add(rulePegText, position118)
				}
				if !_rules[ruleAction15]() {
					goto l97
				}
			l119:
				{
					position120, tokenIndex120 := position, tokenIndex
				l121:
					{
						position122, tokenIndex122 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l122
						}
						goto l121
					l122:
						position, tokenIndex = position122, tokenIndex122
					}
					if buffer[position] != rune(',') {
						goto l120
					}
					position++
				l123:
					{
						position124, tokenIndex124 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l124
						}
						goto l123
					l124:
						position, tokenIndex = position124, tokenIndex124
					}
					{
						position125 := position
						if !_rules[rulereal_column_name]() {
							goto l120
						}
						add(rulePegText, position125)
					}
					if !_rules[ruleAction16]() {
						goto l120
					}
				l126:
					{
						position127, tokenIndex127 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l127
						}
						goto l126
					l127:
						position, tokenIndex = position127, tokenIndex127
					}
					goto l119
				l120:
					position, tokenIndex = position120, tokenIndex120
				}
			l128:
				{
					position129, tokenIndex129 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l129
					}
					goto l128
				l129:
					position, tokenIndex = position129, tokenIndex129
				}
				if buffer[position] != rune(')') {
					goto l97
				}
				position++
				{
					position130, tokenIndex130 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l130
					}
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
					if buffer[position] != rune('u') {
						goto l130
					}
					position++
					if buffer[position] != rune('n') {
						goto l130
					}
					position++
					if buffer[position] != rune('i') {
						goto l130
					}
					position++
					if buffer[position] != rune('q') {
						goto l130
					}
					position++
					if buffer[position] != rune('u') {
						goto l130
					}
					position++
					if buffer[position] != rune('e') {
						goto l130
					}
					position++
					if !_rules[ruleAction17]() {
						goto l130
					}
					goto l131
				l130:
					position, tokenIndex = position130, tokenIndex130
				}
			l131:
			l134:
				{
					position135, tokenIndex135 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l135
					}
					goto l134
				l135:
					position, tokenIndex = position135, tokenIndex135
				}
			l136:
				{
					position137, tokenIndex137 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l137
					}
					goto l136
				l137:
					position, tokenIndex = position137, tokenIndex137
				}
				add(ruleindex_info, position98)
			}
			return true
		l97:
			position, tokenIndex = position97, tokenIndex97
			return false
		},
		/* 12 title <- <(!('\r' / '\n') .)+> */
		func() bool {
			position138, tokenIndex138 := position, tokenIndex
			{
				position139 := position
				{
					position142, tokenIndex142 := position, tokenIndex
					{
						position143, tokenIndex143 := position, tokenIndex
						if buffer[position] != rune('\r') {
							goto l144
						}
						position++
						goto l143
					l144:
						position, tokenIndex = position143, tokenIndex143
						if buffer[position] != rune('\n') {
							goto l142
						}
						position++
					}
				l143:
					goto l138
				l142:
					position, tokenIndex = position142, tokenIndex142
				}
				if !matchDot() {
					goto l138
				}
			l140:
				{
					position141, tokenIndex141 := position, tokenIndex
					{
						position145, tokenIndex145 := position, tokenIndex
						{
							position146, tokenIndex146 := position, tokenIndex
							if buffer[position] != rune('\r') {
								goto l147
							}
							position++
							goto l146
						l147:
							position, tokenIndex = position146, tokenIndex146
							if buffer[position] != rune('\n') {
								goto l145
							}
							position++
						}
					l146:
						goto l141
					l145:
						position, tokenIndex = position145, tokenIndex145
					}
					if !matchDot() {
						goto l141
					}
					goto l140
				l141:
					position, tokenIndex = position141, tokenIndex141
				}
				add(ruletitle, position139)
			}
			return true
		l138:
			position, tokenIndex = position138, tokenIndex138
			return false
		},
		/* 13 comment_string <- <(!('\r' / '\n') .)*> */
		func() bool {
			{
				position149 := position
			l150:
				{
					position151, tokenIndex151 := position, tokenIndex
					{
						position152, tokenIndex152 := position, tokenIndex
						{
							position153, tokenIndex153 := position, tokenIndex
							if buffer[position] != rune('\r') {
								goto l154
							}
							position++
							goto l153
						l154:
							position, tokenIndex = position153, tokenIndex153
							if buffer[position] != rune('\n') {
								goto l152
							}
							position++
						}
					l153:
						goto l151
					l152:
						position, tokenIndex = position152, tokenIndex152
					}
					if !matchDot() {
						goto l151
					}
					goto l150
				l151:
					position, tokenIndex = position151, tokenIndex151
				}
				add(rulecomment_string, position149)
			}
			return true
		},
		/* 14 whitespace <- <(' ' / '\t' / '\r' / '\n')+> */
		func() bool {
			position155, tokenIndex155 := position, tokenIndex
			{
				position156 := position
				{
					position159, tokenIndex159 := position, tokenIndex
					if buffer[position] != rune(' ') {
						goto l160
					}
					position++
					goto l159
				l160:
					position, tokenIndex = position159, tokenIndex159
					if buffer[position] != rune('\t') {
						goto l161
					}
					position++
					goto l159
				l161:
					position, tokenIndex = position159, tokenIndex159
					if buffer[position] != rune('\r') {
						goto l162
					}
					position++
					goto l159
				l162:
					position, tokenIndex = position159, tokenIndex159
					if buffer[position] != rune('\n') {
						goto l155
					}
					position++
				}
			l159:
			l157:
				{
					position158, tokenIndex158 := position, tokenIndex
					{
						position163, tokenIndex163 := position, tokenIndex
						if buffer[position] != rune(' ') {
							goto l164
						}
						position++
						goto l163
					l164:
						position, tokenIndex = position163, tokenIndex163
						if buffer[position] != rune('\t') {
							goto l165
						}
						position++
						goto l163
					l165:
						position, tokenIndex = position163, tokenIndex163
						if buffer[position] != rune('\r') {
							goto l166
						}
						position++
						goto l163
					l166:
						position, tokenIndex = position163, tokenIndex163
						if buffer[position] != rune('\n') {
							goto l158
						}
						position++
					}
				l163:
					goto l157
				l158:
					position, tokenIndex = position158, tokenIndex158
				}
				add(rulewhitespace, position156)
			}
			return true
		l155:
			position, tokenIndex = position155, tokenIndex155
			return false
		},
		/* 15 newline <- <('\r' / '\n')+> */
		func() bool {
			position167, tokenIndex167 := position, tokenIndex
			{
				position168 := position
				{
					position171, tokenIndex171 := position, tokenIndex
					if buffer[position] != rune('\r') {
						goto l172
					}
					position++
					goto l171
				l172:
					position, tokenIndex = position171, tokenIndex171
					if buffer[position] != rune('\n') {
						goto l167
					}
					position++
				}
			l171:
			l169:
				{
					position170, tokenIndex170 := position, tokenIndex
					{
						position173, tokenIndex173 := position, tokenIndex
						if buffer[position] != rune('\r') {
							goto l174
						}
						position++
						goto l173
					l174:
						position, tokenIndex = position173, tokenIndex173
						if buffer[position] != rune('\n') {
							goto l170
						}
						position++
					}
				l173:
					goto l169
				l170:
					position, tokenIndex = position170, tokenIndex170
				}
				add(rulenewline, position168)
			}
			return true
		l167:
			position, tokenIndex = position167, tokenIndex167
			return false
		},
		/* 16 space <- <(' ' / '\t')+> */
		func() bool {
			position175, tokenIndex175 := position, tokenIndex
			{
				position176 := position
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
						goto l175
					}
					position++
				}
			l179:
			l177:
				{
					position178, tokenIndex178 := position, tokenIndex
					{
						position181, tokenIndex181 := position, tokenIndex
						if buffer[position] != rune(' ') {
							goto l182
						}
						position++
						goto l181
					l182:
						position, tokenIndex = position181, tokenIndex181
						if buffer[position] != rune('\t') {
							goto l178
						}
						position++
					}
				l181:
					goto l177
				l178:
					position, tokenIndex = position178, tokenIndex178
				}
				add(rulespace, position176)
			}
			return true
		l175:
			position, tokenIndex = position175, tokenIndex175
			return false
		},
		/* 17 notnull <- <('N' 'N')> */
		func() bool {
			position183, tokenIndex183 := position, tokenIndex
			{
				position184 := position
				if buffer[position] != rune('N') {
					goto l183
				}
				position++
				if buffer[position] != rune('N') {
					goto l183
				}
				position++
				add(rulenotnull, position184)
			}
			return true
		l183:
			position, tokenIndex = position183, tokenIndex183
			return false
		},
		/* 18 unique <- <'U'> */
		func() bool {
			position185, tokenIndex185 := position, tokenIndex
			{
				position186 := position
				if buffer[position] != rune('U') {
					goto l185
				}
				position++
				add(ruleunique, position186)
			}
			return true
		l185:
			position, tokenIndex = position185, tokenIndex185
			return false
		},
		/* 19 real_table_name <- <([a-z] / [A-Z] / [0-9] / '_')+> */
		func() bool {
			position187, tokenIndex187 := position, tokenIndex
			{
				position188 := position
				{
					position191, tokenIndex191 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l192
					}
					position++
					goto l191
				l192:
					position, tokenIndex = position191, tokenIndex191
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l193
					}
					position++
					goto l191
				l193:
					position, tokenIndex = position191, tokenIndex191
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l194
					}
					position++
					goto l191
				l194:
					position, tokenIndex = position191, tokenIndex191
					if buffer[position] != rune('_') {
						goto l187
					}
					position++
				}
			l191:
			l189:
				{
					position190, tokenIndex190 := position, tokenIndex
					{
						position195, tokenIndex195 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l196
						}
						position++
						goto l195
					l196:
						position, tokenIndex = position195, tokenIndex195
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l197
						}
						position++
						goto l195
					l197:
						position, tokenIndex = position195, tokenIndex195
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l198
						}
						position++
						goto l195
					l198:
						position, tokenIndex = position195, tokenIndex195
						if buffer[position] != rune('_') {
							goto l190
						}
						position++
					}
				l195:
					goto l189
				l190:
					position, tokenIndex = position190, tokenIndex190
				}
				add(rulereal_table_name, position188)
			}
			return true
		l187:
			position, tokenIndex = position187, tokenIndex187
			return false
		},
		/* 20 table_name <- <(('"' (!('\t' / '\r' / '\n' / '"') .)+ '"') / (!('\t' / '\r' / '\n' / '/' / ' ') .)+)> */
		func() bool {
			position199, tokenIndex199 := position, tokenIndex
			{
				position200 := position
				{
					position201, tokenIndex201 := position, tokenIndex
					if buffer[position] != rune('"') {
						goto l202
					}
					position++
					{
						position205, tokenIndex205 := position, tokenIndex
						{
							position206, tokenIndex206 := position, tokenIndex
							if buffer[position] != rune('\t') {
								goto l207
							}
							position++
							goto l206
						l207:
							position, tokenIndex = position206, tokenIndex206
							if buffer[position] != rune('\r') {
								goto l208
							}
							position++
							goto l206
						l208:
							position, tokenIndex = position206, tokenIndex206
							if buffer[position] != rune('\n') {
								goto l209
							}
							position++
							goto l206
						l209:
							position, tokenIndex = position206, tokenIndex206
							if buffer[position] != rune('"') {
								goto l205
							}
							position++
						}
					l206:
						goto l202
					l205:
						position, tokenIndex = position205, tokenIndex205
					}
					if !matchDot() {
						goto l202
					}
				l203:
					{
						position204, tokenIndex204 := position, tokenIndex
						{
							position210, tokenIndex210 := position, tokenIndex
							{
								position211, tokenIndex211 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l212
								}
								position++
								goto l211
							l212:
								position, tokenIndex = position211, tokenIndex211
								if buffer[position] != rune('\r') {
									goto l213
								}
								position++
								goto l211
							l213:
								position, tokenIndex = position211, tokenIndex211
								if buffer[position] != rune('\n') {
									goto l214
								}
								position++
								goto l211
							l214:
								position, tokenIndex = position211, tokenIndex211
								if buffer[position] != rune('"') {
									goto l210
								}
								position++
							}
						l211:
							goto l204
						l210:
							position, tokenIndex = position210, tokenIndex210
						}
						if !matchDot() {
							goto l204
						}
						goto l203
					l204:
						position, tokenIndex = position204, tokenIndex204
					}
					if buffer[position] != rune('"') {
						goto l202
					}
					position++
					goto l201
				l202:
					position, tokenIndex = position201, tokenIndex201
					{
						position217, tokenIndex217 := position, tokenIndex
						{
							position218, tokenIndex218 := position, tokenIndex
							if buffer[position] != rune('\t') {
								goto l219
							}
							position++
							goto l218
						l219:
							position, tokenIndex = position218, tokenIndex218
							if buffer[position] != rune('\r') {
								goto l220
							}
							position++
							goto l218
						l220:
							position, tokenIndex = position218, tokenIndex218
							if buffer[position] != rune('\n') {
								goto l221
							}
							position++
							goto l218
						l221:
							position, tokenIndex = position218, tokenIndex218
							if buffer[position] != rune('/') {
								goto l222
							}
							position++
							goto l218
						l222:
							position, tokenIndex = position218, tokenIndex218
							if buffer[position] != rune(' ') {
								goto l217
							}
							position++
						}
					l218:
						goto l199
					l217:
						position, tokenIndex = position217, tokenIndex217
					}
					if !matchDot() {
						goto l199
					}
				l215:
					{
						position216, tokenIndex216 := position, tokenIndex
						{
							position223, tokenIndex223 := position, tokenIndex
							{
								position224, tokenIndex224 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l225
								}
								position++
								goto l224
							l225:
								position, tokenIndex = position224, tokenIndex224
								if buffer[position] != rune('\r') {
									goto l226
								}
								position++
								goto l224
							l226:
								position, tokenIndex = position224, tokenIndex224
								if buffer[position] != rune('\n') {
									goto l227
								}
								position++
								goto l224
							l227:
								position, tokenIndex = position224, tokenIndex224
								if buffer[position] != rune('/') {
									goto l228
								}
								position++
								goto l224
							l228:
								position, tokenIndex = position224, tokenIndex224
								if buffer[position] != rune(' ') {
									goto l223
								}
								position++
							}
						l224:
							goto l216
						l223:
							position, tokenIndex = position223, tokenIndex223
						}
						if !matchDot() {
							goto l216
						}
						goto l215
					l216:
						position, tokenIndex = position216, tokenIndex216
					}
				}
			l201:
				add(ruletable_name, position200)
			}
			return true
		l199:
			position, tokenIndex = position199, tokenIndex199
			return false
		},
		/* 21 real_column_name <- <([a-z] / [A-Z] / [0-9] / '_')+> */
		func() bool {
			position229, tokenIndex229 := position, tokenIndex
			{
				position230 := position
				{
					position233, tokenIndex233 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l234
					}
					position++
					goto l233
				l234:
					position, tokenIndex = position233, tokenIndex233
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l235
					}
					position++
					goto l233
				l235:
					position, tokenIndex = position233, tokenIndex233
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l236
					}
					position++
					goto l233
				l236:
					position, tokenIndex = position233, tokenIndex233
					if buffer[position] != rune('_') {
						goto l229
					}
					position++
				}
			l233:
			l231:
				{
					position232, tokenIndex232 := position, tokenIndex
					{
						position237, tokenIndex237 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l238
						}
						position++
						goto l237
					l238:
						position, tokenIndex = position237, tokenIndex237
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l239
						}
						position++
						goto l237
					l239:
						position, tokenIndex = position237, tokenIndex237
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l240
						}
						position++
						goto l237
					l240:
						position, tokenIndex = position237, tokenIndex237
						if buffer[position] != rune('_') {
							goto l232
						}
						position++
					}
				l237:
					goto l231
				l232:
					position, tokenIndex = position232, tokenIndex232
				}
				add(rulereal_column_name, position230)
			}
			return true
		l229:
			position, tokenIndex = position229, tokenIndex229
			return false
		},
		/* 22 column_name <- <(('"' (!('\t' / '\r' / '\n' / '"') .)+ '"') / (!('\t' / '\r' / '\n' / '/' / ' ') .)+)> */
		func() bool {
			position241, tokenIndex241 := position, tokenIndex
			{
				position242 := position
				{
					position243, tokenIndex243 := position, tokenIndex
					if buffer[position] != rune('"') {
						goto l244
					}
					position++
					{
						position247, tokenIndex247 := position, tokenIndex
						{
							position248, tokenIndex248 := position, tokenIndex
							if buffer[position] != rune('\t') {
								goto l249
							}
							position++
							goto l248
						l249:
							position, tokenIndex = position248, tokenIndex248
							if buffer[position] != rune('\r') {
								goto l250
							}
							position++
							goto l248
						l250:
							position, tokenIndex = position248, tokenIndex248
							if buffer[position] != rune('\n') {
								goto l251
							}
							position++
							goto l248
						l251:
							position, tokenIndex = position248, tokenIndex248
							if buffer[position] != rune('"') {
								goto l247
							}
							position++
						}
					l248:
						goto l244
					l247:
						position, tokenIndex = position247, tokenIndex247
					}
					if !matchDot() {
						goto l244
					}
				l245:
					{
						position246, tokenIndex246 := position, tokenIndex
						{
							position252, tokenIndex252 := position, tokenIndex
							{
								position253, tokenIndex253 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l254
								}
								position++
								goto l253
							l254:
								position, tokenIndex = position253, tokenIndex253
								if buffer[position] != rune('\r') {
									goto l255
								}
								position++
								goto l253
							l255:
								position, tokenIndex = position253, tokenIndex253
								if buffer[position] != rune('\n') {
									goto l256
								}
								position++
								goto l253
							l256:
								position, tokenIndex = position253, tokenIndex253
								if buffer[position] != rune('"') {
									goto l252
								}
								position++
							}
						l253:
							goto l246
						l252:
							position, tokenIndex = position252, tokenIndex252
						}
						if !matchDot() {
							goto l246
						}
						goto l245
					l246:
						position, tokenIndex = position246, tokenIndex246
					}
					if buffer[position] != rune('"') {
						goto l244
					}
					position++
					goto l243
				l244:
					position, tokenIndex = position243, tokenIndex243
					{
						position259, tokenIndex259 := position, tokenIndex
						{
							position260, tokenIndex260 := position, tokenIndex
							if buffer[position] != rune('\t') {
								goto l261
							}
							position++
							goto l260
						l261:
							position, tokenIndex = position260, tokenIndex260
							if buffer[position] != rune('\r') {
								goto l262
							}
							position++
							goto l260
						l262:
							position, tokenIndex = position260, tokenIndex260
							if buffer[position] != rune('\n') {
								goto l263
							}
							position++
							goto l260
						l263:
							position, tokenIndex = position260, tokenIndex260
							if buffer[position] != rune('/') {
								goto l264
							}
							position++
							goto l260
						l264:
							position, tokenIndex = position260, tokenIndex260
							if buffer[position] != rune(' ') {
								goto l259
							}
							position++
						}
					l260:
						goto l241
					l259:
						position, tokenIndex = position259, tokenIndex259
					}
					if !matchDot() {
						goto l241
					}
				l257:
					{
						position258, tokenIndex258 := position, tokenIndex
						{
							position265, tokenIndex265 := position, tokenIndex
							{
								position266, tokenIndex266 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l267
								}
								position++
								goto l266
							l267:
								position, tokenIndex = position266, tokenIndex266
								if buffer[position] != rune('\r') {
									goto l268
								}
								position++
								goto l266
							l268:
								position, tokenIndex = position266, tokenIndex266
								if buffer[position] != rune('\n') {
									goto l269
								}
								position++
								goto l266
							l269:
								position, tokenIndex = position266, tokenIndex266
								if buffer[position] != rune('/') {
									goto l270
								}
								position++
								goto l266
							l270:
								position, tokenIndex = position266, tokenIndex266
								if buffer[position] != rune(' ') {
									goto l265
								}
								position++
							}
						l266:
							goto l258
						l265:
							position, tokenIndex = position265, tokenIndex265
						}
						if !matchDot() {
							goto l258
						}
						goto l257
					l258:
						position, tokenIndex = position258, tokenIndex258
					}
				}
			l243:
				add(rulecolumn_name, position242)
			}
			return true
		l241:
			position, tokenIndex = position241, tokenIndex241
			return false
		},
		/* 23 relation_point <- <([a-z] / [A-Z] / [0-9] / '_' / '.')+> */
		func() bool {
			position271, tokenIndex271 := position, tokenIndex
			{
				position272 := position
				{
					position275, tokenIndex275 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l276
					}
					position++
					goto l275
				l276:
					position, tokenIndex = position275, tokenIndex275
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l277
					}
					position++
					goto l275
				l277:
					position, tokenIndex = position275, tokenIndex275
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l278
					}
					position++
					goto l275
				l278:
					position, tokenIndex = position275, tokenIndex275
					if buffer[position] != rune('_') {
						goto l279
					}
					position++
					goto l275
				l279:
					position, tokenIndex = position275, tokenIndex275
					if buffer[position] != rune('.') {
						goto l271
					}
					position++
				}
			l275:
			l273:
				{
					position274, tokenIndex274 := position, tokenIndex
					{
						position280, tokenIndex280 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l281
						}
						position++
						goto l280
					l281:
						position, tokenIndex = position280, tokenIndex280
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l282
						}
						position++
						goto l280
					l282:
						position, tokenIndex = position280, tokenIndex280
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l283
						}
						position++
						goto l280
					l283:
						position, tokenIndex = position280, tokenIndex280
						if buffer[position] != rune('_') {
							goto l284
						}
						position++
						goto l280
					l284:
						position, tokenIndex = position280, tokenIndex280
						if buffer[position] != rune('.') {
							goto l274
						}
						position++
					}
				l280:
					goto l273
				l274:
					position, tokenIndex = position274, tokenIndex274
				}
				add(rulerelation_point, position272)
			}
			return true
		l271:
			position, tokenIndex = position271, tokenIndex271
			return false
		},
		/* 24 pkey <- <('+' / '*')> */
		func() bool {
			position285, tokenIndex285 := position, tokenIndex
			{
				position286 := position
				{
					position287, tokenIndex287 := position, tokenIndex
					if buffer[position] != rune('+') {
						goto l288
					}
					position++
					goto l287
				l288:
					position, tokenIndex = position287, tokenIndex287
					if buffer[position] != rune('*') {
						goto l285
					}
					position++
				}
			l287:
				add(rulepkey, position286)
			}
			return true
		l285:
			position, tokenIndex = position285, tokenIndex285
			return false
		},
		/* 25 col_type <- <([a-z] / [A-Z] / [0-9] / '_' / '(' / ')' / ' ' / '.' / ',')+> */
		func() bool {
			position289, tokenIndex289 := position, tokenIndex
			{
				position290 := position
				{
					position293, tokenIndex293 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l294
					}
					position++
					goto l293
				l294:
					position, tokenIndex = position293, tokenIndex293
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l295
					}
					position++
					goto l293
				l295:
					position, tokenIndex = position293, tokenIndex293
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l296
					}
					position++
					goto l293
				l296:
					position, tokenIndex = position293, tokenIndex293
					if buffer[position] != rune('_') {
						goto l297
					}
					position++
					goto l293
				l297:
					position, tokenIndex = position293, tokenIndex293
					if buffer[position] != rune('(') {
						goto l298
					}
					position++
					goto l293
				l298:
					position, tokenIndex = position293, tokenIndex293
					if buffer[position] != rune(')') {
						goto l299
					}
					position++
					goto l293
				l299:
					position, tokenIndex = position293, tokenIndex293
					if buffer[position] != rune(' ') {
						goto l300
					}
					position++
					goto l293
				l300:
					position, tokenIndex = position293, tokenIndex293
					if buffer[position] != rune('.') {
						goto l301
					}
					position++
					goto l293
				l301:
					position, tokenIndex = position293, tokenIndex293
					if buffer[position] != rune(',') {
						goto l289
					}
					position++
				}
			l293:
			l291:
				{
					position292, tokenIndex292 := position, tokenIndex
					{
						position302, tokenIndex302 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l303
						}
						position++
						goto l302
					l303:
						position, tokenIndex = position302, tokenIndex302
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l304
						}
						position++
						goto l302
					l304:
						position, tokenIndex = position302, tokenIndex302
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l305
						}
						position++
						goto l302
					l305:
						position, tokenIndex = position302, tokenIndex302
						if buffer[position] != rune('_') {
							goto l306
						}
						position++
						goto l302
					l306:
						position, tokenIndex = position302, tokenIndex302
						if buffer[position] != rune('(') {
							goto l307
						}
						position++
						goto l302
					l307:
						position, tokenIndex = position302, tokenIndex302
						if buffer[position] != rune(')') {
							goto l308
						}
						position++
						goto l302
					l308:
						position, tokenIndex = position302, tokenIndex302
						if buffer[position] != rune(' ') {
							goto l309
						}
						position++
						goto l302
					l309:
						position, tokenIndex = position302, tokenIndex302
						if buffer[position] != rune('.') {
							goto l310
						}
						position++
						goto l302
					l310:
						position, tokenIndex = position302, tokenIndex302
						if buffer[position] != rune(',') {
							goto l292
						}
						position++
					}
				l302:
					goto l291
				l292:
					position, tokenIndex = position292, tokenIndex292
				}
				add(rulecol_type, position290)
			}
			return true
		l289:
			position, tokenIndex = position289, tokenIndex289
			return false
		},
		/* 26 default <- <((!('\r' / '\n' / ']') .) / ('\\' ']'))*> */
		func() bool {
			{
				position312 := position
			l313:
				{
					position314, tokenIndex314 := position, tokenIndex
					{
						position315, tokenIndex315 := position, tokenIndex
						{
							position317, tokenIndex317 := position, tokenIndex
							{
								position318, tokenIndex318 := position, tokenIndex
								if buffer[position] != rune('\r') {
									goto l319
								}
								position++
								goto l318
							l319:
								position, tokenIndex = position318, tokenIndex318
								if buffer[position] != rune('\n') {
									goto l320
								}
								position++
								goto l318
							l320:
								position, tokenIndex = position318, tokenIndex318
								if buffer[position] != rune(']') {
									goto l317
								}
								position++
							}
						l318:
							goto l316
						l317:
							position, tokenIndex = position317, tokenIndex317
						}
						if !matchDot() {
							goto l316
						}
						goto l315
					l316:
						position, tokenIndex = position315, tokenIndex315
						if buffer[position] != rune('\\') {
							goto l314
						}
						position++
						if buffer[position] != rune(']') {
							goto l314
						}
						position++
					}
				l315:
					goto l313
				l314:
					position, tokenIndex = position314, tokenIndex314
				}
				add(ruledefault, position312)
			}
			return true
		},
		/* 27 cardinality_right <- <cardinality> */
		func() bool {
			position321, tokenIndex321 := position, tokenIndex
			{
				position322 := position
				if !_rules[rulecardinality]() {
					goto l321
				}
				add(rulecardinality_right, position322)
			}
			return true
		l321:
			position, tokenIndex = position321, tokenIndex321
			return false
		},
		/* 28 cardinality_left <- <cardinality> */
		func() bool {
			position323, tokenIndex323 := position, tokenIndex
			{
				position324 := position
				if !_rules[rulecardinality]() {
					goto l323
				}
				add(rulecardinality_left, position324)
			}
			return true
		l323:
			position, tokenIndex = position323, tokenIndex323
			return false
		},
		/* 29 cardinality <- <(('0' / '1' / '*') (. . ('0' / '1' / '*'))?)> */
		func() bool {
			position325, tokenIndex325 := position, tokenIndex
			{
				position326 := position
				{
					position327, tokenIndex327 := position, tokenIndex
					if buffer[position] != rune('0') {
						goto l328
					}
					position++
					goto l327
				l328:
					position, tokenIndex = position327, tokenIndex327
					if buffer[position] != rune('1') {
						goto l329
					}
					position++
					goto l327
				l329:
					position, tokenIndex = position327, tokenIndex327
					if buffer[position] != rune('*') {
						goto l325
					}
					position++
				}
			l327:
				{
					position330, tokenIndex330 := position, tokenIndex
					if !matchDot() {
						goto l330
					}
					if !matchDot() {
						goto l330
					}
					{
						position332, tokenIndex332 := position, tokenIndex
						if buffer[position] != rune('0') {
							goto l333
						}
						position++
						goto l332
					l333:
						position, tokenIndex = position332, tokenIndex332
						if buffer[position] != rune('1') {
							goto l334
						}
						position++
						goto l332
					l334:
						position, tokenIndex = position332, tokenIndex332
						if buffer[position] != rune('*') {
							goto l330
						}
						position++
					}
				l332:
					goto l331
				l330:
					position, tokenIndex = position330, tokenIndex330
				}
			l331:
				add(rulecardinality, position326)
			}
			return true
		l325:
			position, tokenIndex = position325, tokenIndex325
			return false
		},
		nil,
		/* 32 Action0 <- <{p.setTitle(text)}> */
		func() bool {
			{
				add(ruleAction0, position)
			}
			return true
		},
		/* 33 Action1 <- <{p.addTableTitleReal(text)}> */
		func() bool {
			{
				add(ruleAction1, position)
			}
			return true
		},
		/* 34 Action2 <- <{p.addTableTitle(text)}> */
		func() bool {
			{
				add(ruleAction2, position)
			}
			return true
		},
		/* 35 Action3 <- <{ p.addPrimaryKey(text) }> */
		func() bool {
			{
				add(ruleAction3, position)
			}
			return true
		},
		/* 36 Action4 <- <{ p.setColumnNameReal(text) }> */
		func() bool {
			{
				add(ruleAction4, position)
			}
			return true
		},
		/* 37 Action5 <- <{ p.setColumnName(text) }> */
		func() bool {
			{
				add(ruleAction5, position)
			}
			return true
		},
		/* 38 Action6 <- <{ p.addColumnType(text) }> */
		func() bool {
			{
				add(ruleAction6, position)
			}
			return true
		},
		/* 39 Action7 <- <{ p.setNotNull() }> */
		func() bool {
			{
				add(ruleAction7, position)
			}
			return true
		},
		/* 40 Action8 <- <{ p.setUnique() }> */
		func() bool {
			{
				add(ruleAction8, position)
			}
			return true
		},
		/* 41 Action9 <- <{ p.setColumnDefault(text) }> */
		func() bool {
			{
				add(ruleAction9, position)
			}
			return true
		},
		/* 42 Action10 <- <{ p.setRelationSource(text) }> */
		func() bool {
			{
				add(ruleAction10, position)
			}
			return true
		},
		/* 43 Action11 <- <{ p.setRelationDestination(text) }> */
		func() bool {
			{
				add(ruleAction11, position)
			}
			return true
		},
		/* 44 Action12 <- <{ p.setRelationTableNameReal(text) }> */
		func() bool {
			{
				add(ruleAction12, position)
			}
			return true
		},
		/* 45 Action13 <- <{ p.addComment(text) }> */
		func() bool {
			{
				add(ruleAction13, position)
			}
			return true
		},
		/* 46 Action14 <- <{p.setIndexName(text)}> */
		func() bool {
			{
				add(ruleAction14, position)
			}
			return true
		},
		/* 47 Action15 <- <{p.setIndexColumn(text)}> */
		func() bool {
			{
				add(ruleAction15, position)
			}
			return true
		},
		/* 48 Action16 <- <{p.setIndexColumn(text)}> */
		func() bool {
			{
				add(ruleAction16, position)
			}
			return true
		},
		/* 49 Action17 <- <{ p.setUniqueIndex() }> */
		func() bool {
			{
				add(ruleAction17, position)
			}
			return true
		},
	}
	p.rules = _rules
}
