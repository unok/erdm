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
	ruleerd
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
	ruleAction20
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
	"erd",
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
	"Action20",
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
	rules  [55]func() bool
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
			p.setWithoutErd()
		case ruleAction13:
			p.setRelationSource(text)
		case ruleAction14:
			p.setRelationDestination(text)
		case ruleAction15:
			p.setRelationTableNameReal(text)
		case ruleAction16:
			p.addComment(text)
		case ruleAction17:
			p.setIndexName(text)
		case ruleAction18:
			p.setIndexColumn(text)
		case ruleAction19:
			p.setIndexColumn(text)
		case ruleAction20:
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
		/* 7 table_name_info <- <(<real_table_name> Action3 space* ('/' space* <table_name> Action4)? space* newline*)> */
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
				{
					position45, tokenIndex45 := position, tokenIndex
					if buffer[position] != rune('/') {
						goto l45
					}
					position++
				l47:
					{
						position48, tokenIndex48 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l48
						}
						goto l47
					l48:
						position, tokenIndex = position48, tokenIndex48
					}
					{
						position49 := position
						if !_rules[ruletable_name]() {
							goto l45
						}
						add(rulePegText, position49)
					}
					if !_rules[ruleAction4]() {
						goto l45
					}
					goto l46
				l45:
					position, tokenIndex = position45, tokenIndex45
				}
			l46:
			l50:
				{
					position51, tokenIndex51 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l51
					}
					goto l50
				l51:
					position, tokenIndex = position51, tokenIndex51
				}
			l52:
				{
					position53, tokenIndex53 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l53
					}
					goto l52
				l53:
					position, tokenIndex = position53, tokenIndex53
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
			position54, tokenIndex54 := position, tokenIndex
			{
				position55 := position
				if !_rules[rulecolumn_attribute]() {
					goto l54
				}
				{
					position56, tokenIndex56 := position, tokenIndex
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
					if !_rules[rulerelation]() {
						goto l56
					}
				l60:
					{
						position61, tokenIndex61 := position, tokenIndex
					l62:
						{
							position63, tokenIndex63 := position, tokenIndex
							if !_rules[rulespace]() {
								goto l63
							}
							goto l62
						l63:
							position, tokenIndex = position63, tokenIndex63
						}
						if !_rules[rulerelation]() {
							goto l61
						}
						goto l60
					l61:
						position, tokenIndex = position61, tokenIndex61
					}
					goto l57
				l56:
					position, tokenIndex = position56, tokenIndex56
				}
			l57:
			l64:
				{
					position65, tokenIndex65 := position, tokenIndex
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
					if !_rules[rulecolumn_comment]() {
						goto l65
					}
					goto l64
				l65:
					position, tokenIndex = position65, tokenIndex65
				}
				{
					position68, tokenIndex68 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l68
					}
					goto l69
				l68:
					position, tokenIndex = position68, tokenIndex68
				}
			l69:
				add(rulecolumn_info, position55)
			}
			return true
		l54:
			position, tokenIndex = position54, tokenIndex54
			return false
		},
		/* 9 column_attribute <- <(space+ (<pkey> Action5)? <real_column_name> Action6 ('/' <column_name> Action7)? space+ '[' <col_type> Action8 ']' (('[' notnull Action9 ']') / ('[' unique Action10 ']') / ('[' '=' <default> Action11 ']') / ('[' <erd> Action12 ']'))* newline?)> */
		func() bool {
			position70, tokenIndex70 := position, tokenIndex
			{
				position71 := position
				if !_rules[rulespace]() {
					goto l70
				}
			l72:
				{
					position73, tokenIndex73 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l73
					}
					goto l72
				l73:
					position, tokenIndex = position73, tokenIndex73
				}
				{
					position74, tokenIndex74 := position, tokenIndex
					{
						position76 := position
						if !_rules[rulepkey]() {
							goto l74
						}
						add(rulePegText, position76)
					}
					if !_rules[ruleAction5]() {
						goto l74
					}
					goto l75
				l74:
					position, tokenIndex = position74, tokenIndex74
				}
			l75:
				{
					position77 := position
					if !_rules[rulereal_column_name]() {
						goto l70
					}
					add(rulePegText, position77)
				}
				if !_rules[ruleAction6]() {
					goto l70
				}
				{
					position78, tokenIndex78 := position, tokenIndex
					if buffer[position] != rune('/') {
						goto l78
					}
					position++
					{
						position80 := position
						if !_rules[rulecolumn_name]() {
							goto l78
						}
						add(rulePegText, position80)
					}
					if !_rules[ruleAction7]() {
						goto l78
					}
					goto l79
				l78:
					position, tokenIndex = position78, tokenIndex78
				}
			l79:
				if !_rules[rulespace]() {
					goto l70
				}
			l81:
				{
					position82, tokenIndex82 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l82
					}
					goto l81
				l82:
					position, tokenIndex = position82, tokenIndex82
				}
				if buffer[position] != rune('[') {
					goto l70
				}
				position++
				{
					position83 := position
					if !_rules[rulecol_type]() {
						goto l70
					}
					add(rulePegText, position83)
				}
				if !_rules[ruleAction8]() {
					goto l70
				}
				if buffer[position] != rune(']') {
					goto l70
				}
				position++
			l84:
				{
					position85, tokenIndex85 := position, tokenIndex
					{
						position86, tokenIndex86 := position, tokenIndex
						if buffer[position] != rune('[') {
							goto l87
						}
						position++
						if !_rules[rulenotnull]() {
							goto l87
						}
						if !_rules[ruleAction9]() {
							goto l87
						}
						if buffer[position] != rune(']') {
							goto l87
						}
						position++
						goto l86
					l87:
						position, tokenIndex = position86, tokenIndex86
						if buffer[position] != rune('[') {
							goto l88
						}
						position++
						if !_rules[ruleunique]() {
							goto l88
						}
						if !_rules[ruleAction10]() {
							goto l88
						}
						if buffer[position] != rune(']') {
							goto l88
						}
						position++
						goto l86
					l88:
						position, tokenIndex = position86, tokenIndex86
						if buffer[position] != rune('[') {
							goto l89
						}
						position++
						if buffer[position] != rune('=') {
							goto l89
						}
						position++
						{
							position90 := position
							if !_rules[ruledefault]() {
								goto l89
							}
							add(rulePegText, position90)
						}
						if !_rules[ruleAction11]() {
							goto l89
						}
						if buffer[position] != rune(']') {
							goto l89
						}
						position++
						goto l86
					l89:
						position, tokenIndex = position86, tokenIndex86
						if buffer[position] != rune('[') {
							goto l85
						}
						position++
						{
							position91 := position
							if !_rules[ruleerd]() {
								goto l85
							}
							add(rulePegText, position91)
						}
						if !_rules[ruleAction12]() {
							goto l85
						}
						if buffer[position] != rune(']') {
							goto l85
						}
						position++
					}
				l86:
					goto l84
				l85:
					position, tokenIndex = position85, tokenIndex85
				}
				{
					position92, tokenIndex92 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l92
					}
					goto l93
				l92:
					position, tokenIndex = position92, tokenIndex92
				}
			l93:
				add(rulecolumn_attribute, position71)
			}
			return true
		l70:
			position, tokenIndex = position70, tokenIndex70
			return false
		},
		/* 10 relation <- <((<cardinality_left> Action13)? space* ('-' '-') space* (<cardinality_right> Action14 space)? space* <relation_point> Action15)> */
		func() bool {
			position94, tokenIndex94 := position, tokenIndex
			{
				position95 := position
				{
					position96, tokenIndex96 := position, tokenIndex
					{
						position98 := position
						if !_rules[rulecardinality_left]() {
							goto l96
						}
						add(rulePegText, position98)
					}
					if !_rules[ruleAction13]() {
						goto l96
					}
					goto l97
				l96:
					position, tokenIndex = position96, tokenIndex96
				}
			l97:
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
				if buffer[position] != rune('-') {
					goto l94
				}
				position++
				if buffer[position] != rune('-') {
					goto l94
				}
				position++
			l101:
				{
					position102, tokenIndex102 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l102
					}
					goto l101
				l102:
					position, tokenIndex = position102, tokenIndex102
				}
				{
					position103, tokenIndex103 := position, tokenIndex
					{
						position105 := position
						if !_rules[rulecardinality_right]() {
							goto l103
						}
						add(rulePegText, position105)
					}
					if !_rules[ruleAction14]() {
						goto l103
					}
					if !_rules[rulespace]() {
						goto l103
					}
					goto l104
				l103:
					position, tokenIndex = position103, tokenIndex103
				}
			l104:
			l106:
				{
					position107, tokenIndex107 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l107
					}
					goto l106
				l107:
					position, tokenIndex = position107, tokenIndex107
				}
				{
					position108 := position
					if !_rules[rulerelation_point]() {
						goto l94
					}
					add(rulePegText, position108)
				}
				if !_rules[ruleAction15]() {
					goto l94
				}
				add(rulerelation, position95)
			}
			return true
		l94:
			position, tokenIndex = position94, tokenIndex94
			return false
		},
		/* 11 column_comment <- <(space+ '#' space? <comment_string> Action16)> */
		func() bool {
			position109, tokenIndex109 := position, tokenIndex
			{
				position110 := position
				if !_rules[rulespace]() {
					goto l109
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
				if buffer[position] != rune('#') {
					goto l109
				}
				position++
				{
					position113, tokenIndex113 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l113
					}
					goto l114
				l113:
					position, tokenIndex = position113, tokenIndex113
				}
			l114:
				{
					position115 := position
					if !_rules[rulecomment_string]() {
						goto l109
					}
					add(rulePegText, position115)
				}
				if !_rules[ruleAction16]() {
					goto l109
				}
				add(rulecolumn_comment, position110)
			}
			return true
		l109:
			position, tokenIndex = position109, tokenIndex109
			return false
		},
		/* 12 index_info <- <(space+ (('i' / 'I') ('n' / 'N') ('d' / 'D') ('e' / 'E') ('x' / 'X')) space+ <real_column_name> Action17 space+ '(' space* <real_column_name> Action18 (space* ',' space* <real_column_name> Action19 space*)* space* ')' (space+ ('u' 'n' 'i' 'q' 'u' 'e') Action20)? space* newline*)> */
		func() bool {
			position116, tokenIndex116 := position, tokenIndex
			{
				position117 := position
				if !_rules[rulespace]() {
					goto l116
				}
			l118:
				{
					position119, tokenIndex119 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l119
					}
					goto l118
				l119:
					position, tokenIndex = position119, tokenIndex119
				}
				{
					position120, tokenIndex120 := position, tokenIndex
					if buffer[position] != rune('i') {
						goto l121
					}
					position++
					goto l120
				l121:
					position, tokenIndex = position120, tokenIndex120
					if buffer[position] != rune('I') {
						goto l116
					}
					position++
				}
			l120:
				{
					position122, tokenIndex122 := position, tokenIndex
					if buffer[position] != rune('n') {
						goto l123
					}
					position++
					goto l122
				l123:
					position, tokenIndex = position122, tokenIndex122
					if buffer[position] != rune('N') {
						goto l116
					}
					position++
				}
			l122:
				{
					position124, tokenIndex124 := position, tokenIndex
					if buffer[position] != rune('d') {
						goto l125
					}
					position++
					goto l124
				l125:
					position, tokenIndex = position124, tokenIndex124
					if buffer[position] != rune('D') {
						goto l116
					}
					position++
				}
			l124:
				{
					position126, tokenIndex126 := position, tokenIndex
					if buffer[position] != rune('e') {
						goto l127
					}
					position++
					goto l126
				l127:
					position, tokenIndex = position126, tokenIndex126
					if buffer[position] != rune('E') {
						goto l116
					}
					position++
				}
			l126:
				{
					position128, tokenIndex128 := position, tokenIndex
					if buffer[position] != rune('x') {
						goto l129
					}
					position++
					goto l128
				l129:
					position, tokenIndex = position128, tokenIndex128
					if buffer[position] != rune('X') {
						goto l116
					}
					position++
				}
			l128:
				if !_rules[rulespace]() {
					goto l116
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
				{
					position132 := position
					if !_rules[rulereal_column_name]() {
						goto l116
					}
					add(rulePegText, position132)
				}
				if !_rules[ruleAction17]() {
					goto l116
				}
				if !_rules[rulespace]() {
					goto l116
				}
			l133:
				{
					position134, tokenIndex134 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l134
					}
					goto l133
				l134:
					position, tokenIndex = position134, tokenIndex134
				}
				if buffer[position] != rune('(') {
					goto l116
				}
				position++
			l135:
				{
					position136, tokenIndex136 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l136
					}
					goto l135
				l136:
					position, tokenIndex = position136, tokenIndex136
				}
				{
					position137 := position
					if !_rules[rulereal_column_name]() {
						goto l116
					}
					add(rulePegText, position137)
				}
				if !_rules[ruleAction18]() {
					goto l116
				}
			l138:
				{
					position139, tokenIndex139 := position, tokenIndex
				l140:
					{
						position141, tokenIndex141 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l141
						}
						goto l140
					l141:
						position, tokenIndex = position141, tokenIndex141
					}
					if buffer[position] != rune(',') {
						goto l139
					}
					position++
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
					{
						position144 := position
						if !_rules[rulereal_column_name]() {
							goto l139
						}
						add(rulePegText, position144)
					}
					if !_rules[ruleAction19]() {
						goto l139
					}
				l145:
					{
						position146, tokenIndex146 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l146
						}
						goto l145
					l146:
						position, tokenIndex = position146, tokenIndex146
					}
					goto l138
				l139:
					position, tokenIndex = position139, tokenIndex139
				}
			l147:
				{
					position148, tokenIndex148 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l148
					}
					goto l147
				l148:
					position, tokenIndex = position148, tokenIndex148
				}
				if buffer[position] != rune(')') {
					goto l116
				}
				position++
				{
					position149, tokenIndex149 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l149
					}
				l151:
					{
						position152, tokenIndex152 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l152
						}
						goto l151
					l152:
						position, tokenIndex = position152, tokenIndex152
					}
					if buffer[position] != rune('u') {
						goto l149
					}
					position++
					if buffer[position] != rune('n') {
						goto l149
					}
					position++
					if buffer[position] != rune('i') {
						goto l149
					}
					position++
					if buffer[position] != rune('q') {
						goto l149
					}
					position++
					if buffer[position] != rune('u') {
						goto l149
					}
					position++
					if buffer[position] != rune('e') {
						goto l149
					}
					position++
					if !_rules[ruleAction20]() {
						goto l149
					}
					goto l150
				l149:
					position, tokenIndex = position149, tokenIndex149
				}
			l150:
			l153:
				{
					position154, tokenIndex154 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l154
					}
					goto l153
				l154:
					position, tokenIndex = position154, tokenIndex154
				}
			l155:
				{
					position156, tokenIndex156 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l156
					}
					goto l155
				l156:
					position, tokenIndex = position156, tokenIndex156
				}
				add(ruleindex_info, position117)
			}
			return true
		l116:
			position, tokenIndex = position116, tokenIndex116
			return false
		},
		/* 13 title <- <(!('\r' / '\n') .)+> */
		func() bool {
			position157, tokenIndex157 := position, tokenIndex
			{
				position158 := position
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
			l159:
				{
					position160, tokenIndex160 := position, tokenIndex
					{
						position164, tokenIndex164 := position, tokenIndex
						{
							position165, tokenIndex165 := position, tokenIndex
							if buffer[position] != rune('\r') {
								goto l166
							}
							position++
							goto l165
						l166:
							position, tokenIndex = position165, tokenIndex165
							if buffer[position] != rune('\n') {
								goto l164
							}
							position++
						}
					l165:
						goto l160
					l164:
						position, tokenIndex = position164, tokenIndex164
					}
					if !matchDot() {
						goto l160
					}
					goto l159
				l160:
					position, tokenIndex = position160, tokenIndex160
				}
				add(ruletitle, position158)
			}
			return true
		l157:
			position, tokenIndex = position157, tokenIndex157
			return false
		},
		/* 14 comment_string <- <(!('\r' / '\n') .)*> */
		func() bool {
			{
				position168 := position
			l169:
				{
					position170, tokenIndex170 := position, tokenIndex
					{
						position171, tokenIndex171 := position, tokenIndex
						{
							position172, tokenIndex172 := position, tokenIndex
							if buffer[position] != rune('\r') {
								goto l173
							}
							position++
							goto l172
						l173:
							position, tokenIndex = position172, tokenIndex172
							if buffer[position] != rune('\n') {
								goto l171
							}
							position++
						}
					l172:
						goto l170
					l171:
						position, tokenIndex = position171, tokenIndex171
					}
					if !matchDot() {
						goto l170
					}
					goto l169
				l170:
					position, tokenIndex = position170, tokenIndex170
				}
				add(rulecomment_string, position168)
			}
			return true
		},
		/* 15 whitespace <- <(' ' / '\t' / '\r' / '\n')+> */
		func() bool {
			position174, tokenIndex174 := position, tokenIndex
			{
				position175 := position
				{
					position178, tokenIndex178 := position, tokenIndex
					if buffer[position] != rune(' ') {
						goto l179
					}
					position++
					goto l178
				l179:
					position, tokenIndex = position178, tokenIndex178
					if buffer[position] != rune('\t') {
						goto l180
					}
					position++
					goto l178
				l180:
					position, tokenIndex = position178, tokenIndex178
					if buffer[position] != rune('\r') {
						goto l181
					}
					position++
					goto l178
				l181:
					position, tokenIndex = position178, tokenIndex178
					if buffer[position] != rune('\n') {
						goto l174
					}
					position++
				}
			l178:
			l176:
				{
					position177, tokenIndex177 := position, tokenIndex
					{
						position182, tokenIndex182 := position, tokenIndex
						if buffer[position] != rune(' ') {
							goto l183
						}
						position++
						goto l182
					l183:
						position, tokenIndex = position182, tokenIndex182
						if buffer[position] != rune('\t') {
							goto l184
						}
						position++
						goto l182
					l184:
						position, tokenIndex = position182, tokenIndex182
						if buffer[position] != rune('\r') {
							goto l185
						}
						position++
						goto l182
					l185:
						position, tokenIndex = position182, tokenIndex182
						if buffer[position] != rune('\n') {
							goto l177
						}
						position++
					}
				l182:
					goto l176
				l177:
					position, tokenIndex = position177, tokenIndex177
				}
				add(rulewhitespace, position175)
			}
			return true
		l174:
			position, tokenIndex = position174, tokenIndex174
			return false
		},
		/* 16 newline <- <('\r' / '\n')+> */
		func() bool {
			position186, tokenIndex186 := position, tokenIndex
			{
				position187 := position
				{
					position190, tokenIndex190 := position, tokenIndex
					if buffer[position] != rune('\r') {
						goto l191
					}
					position++
					goto l190
				l191:
					position, tokenIndex = position190, tokenIndex190
					if buffer[position] != rune('\n') {
						goto l186
					}
					position++
				}
			l190:
			l188:
				{
					position189, tokenIndex189 := position, tokenIndex
					{
						position192, tokenIndex192 := position, tokenIndex
						if buffer[position] != rune('\r') {
							goto l193
						}
						position++
						goto l192
					l193:
						position, tokenIndex = position192, tokenIndex192
						if buffer[position] != rune('\n') {
							goto l189
						}
						position++
					}
				l192:
					goto l188
				l189:
					position, tokenIndex = position189, tokenIndex189
				}
				add(rulenewline, position187)
			}
			return true
		l186:
			position, tokenIndex = position186, tokenIndex186
			return false
		},
		/* 17 space <- <(' ' / '\t')+> */
		func() bool {
			position194, tokenIndex194 := position, tokenIndex
			{
				position195 := position
				{
					position198, tokenIndex198 := position, tokenIndex
					if buffer[position] != rune(' ') {
						goto l199
					}
					position++
					goto l198
				l199:
					position, tokenIndex = position198, tokenIndex198
					if buffer[position] != rune('\t') {
						goto l194
					}
					position++
				}
			l198:
			l196:
				{
					position197, tokenIndex197 := position, tokenIndex
					{
						position200, tokenIndex200 := position, tokenIndex
						if buffer[position] != rune(' ') {
							goto l201
						}
						position++
						goto l200
					l201:
						position, tokenIndex = position200, tokenIndex200
						if buffer[position] != rune('\t') {
							goto l197
						}
						position++
					}
				l200:
					goto l196
				l197:
					position, tokenIndex = position197, tokenIndex197
				}
				add(rulespace, position195)
			}
			return true
		l194:
			position, tokenIndex = position194, tokenIndex194
			return false
		},
		/* 18 notnull <- <('N' 'N')> */
		func() bool {
			position202, tokenIndex202 := position, tokenIndex
			{
				position203 := position
				if buffer[position] != rune('N') {
					goto l202
				}
				position++
				if buffer[position] != rune('N') {
					goto l202
				}
				position++
				add(rulenotnull, position203)
			}
			return true
		l202:
			position, tokenIndex = position202, tokenIndex202
			return false
		},
		/* 19 unique <- <'U'> */
		func() bool {
			position204, tokenIndex204 := position, tokenIndex
			{
				position205 := position
				if buffer[position] != rune('U') {
					goto l204
				}
				position++
				add(ruleunique, position205)
			}
			return true
		l204:
			position, tokenIndex = position204, tokenIndex204
			return false
		},
		/* 20 erd <- <('-' 'e' 'r' 'd')> */
		func() bool {
			position206, tokenIndex206 := position, tokenIndex
			{
				position207 := position
				if buffer[position] != rune('-') {
					goto l206
				}
				position++
				if buffer[position] != rune('e') {
					goto l206
				}
				position++
				if buffer[position] != rune('r') {
					goto l206
				}
				position++
				if buffer[position] != rune('d') {
					goto l206
				}
				position++
				add(ruleerd, position207)
			}
			return true
		l206:
			position, tokenIndex = position206, tokenIndex206
			return false
		},
		/* 21 real_table_name <- <([a-z] / [A-Z] / [0-9] / '_')+> */
		func() bool {
			position208, tokenIndex208 := position, tokenIndex
			{
				position209 := position
				{
					position212, tokenIndex212 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l213
					}
					position++
					goto l212
				l213:
					position, tokenIndex = position212, tokenIndex212
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l214
					}
					position++
					goto l212
				l214:
					position, tokenIndex = position212, tokenIndex212
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l215
					}
					position++
					goto l212
				l215:
					position, tokenIndex = position212, tokenIndex212
					if buffer[position] != rune('_') {
						goto l208
					}
					position++
				}
			l212:
			l210:
				{
					position211, tokenIndex211 := position, tokenIndex
					{
						position216, tokenIndex216 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l217
						}
						position++
						goto l216
					l217:
						position, tokenIndex = position216, tokenIndex216
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l218
						}
						position++
						goto l216
					l218:
						position, tokenIndex = position216, tokenIndex216
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l219
						}
						position++
						goto l216
					l219:
						position, tokenIndex = position216, tokenIndex216
						if buffer[position] != rune('_') {
							goto l211
						}
						position++
					}
				l216:
					goto l210
				l211:
					position, tokenIndex = position211, tokenIndex211
				}
				add(rulereal_table_name, position209)
			}
			return true
		l208:
			position, tokenIndex = position208, tokenIndex208
			return false
		},
		/* 22 table_name <- <(('"' (!('\t' / '\r' / '\n' / '"') .)+ '"') / (!('\t' / '\r' / '\n' / '/' / ' ') .)+)> */
		func() bool {
			position220, tokenIndex220 := position, tokenIndex
			{
				position221 := position
				{
					position222, tokenIndex222 := position, tokenIndex
					if buffer[position] != rune('"') {
						goto l223
					}
					position++
					{
						position226, tokenIndex226 := position, tokenIndex
						{
							position227, tokenIndex227 := position, tokenIndex
							if buffer[position] != rune('\t') {
								goto l228
							}
							position++
							goto l227
						l228:
							position, tokenIndex = position227, tokenIndex227
							if buffer[position] != rune('\r') {
								goto l229
							}
							position++
							goto l227
						l229:
							position, tokenIndex = position227, tokenIndex227
							if buffer[position] != rune('\n') {
								goto l230
							}
							position++
							goto l227
						l230:
							position, tokenIndex = position227, tokenIndex227
							if buffer[position] != rune('"') {
								goto l226
							}
							position++
						}
					l227:
						goto l223
					l226:
						position, tokenIndex = position226, tokenIndex226
					}
					if !matchDot() {
						goto l223
					}
				l224:
					{
						position225, tokenIndex225 := position, tokenIndex
						{
							position231, tokenIndex231 := position, tokenIndex
							{
								position232, tokenIndex232 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l233
								}
								position++
								goto l232
							l233:
								position, tokenIndex = position232, tokenIndex232
								if buffer[position] != rune('\r') {
									goto l234
								}
								position++
								goto l232
							l234:
								position, tokenIndex = position232, tokenIndex232
								if buffer[position] != rune('\n') {
									goto l235
								}
								position++
								goto l232
							l235:
								position, tokenIndex = position232, tokenIndex232
								if buffer[position] != rune('"') {
									goto l231
								}
								position++
							}
						l232:
							goto l225
						l231:
							position, tokenIndex = position231, tokenIndex231
						}
						if !matchDot() {
							goto l225
						}
						goto l224
					l225:
						position, tokenIndex = position225, tokenIndex225
					}
					if buffer[position] != rune('"') {
						goto l223
					}
					position++
					goto l222
				l223:
					position, tokenIndex = position222, tokenIndex222
					{
						position238, tokenIndex238 := position, tokenIndex
						{
							position239, tokenIndex239 := position, tokenIndex
							if buffer[position] != rune('\t') {
								goto l240
							}
							position++
							goto l239
						l240:
							position, tokenIndex = position239, tokenIndex239
							if buffer[position] != rune('\r') {
								goto l241
							}
							position++
							goto l239
						l241:
							position, tokenIndex = position239, tokenIndex239
							if buffer[position] != rune('\n') {
								goto l242
							}
							position++
							goto l239
						l242:
							position, tokenIndex = position239, tokenIndex239
							if buffer[position] != rune('/') {
								goto l243
							}
							position++
							goto l239
						l243:
							position, tokenIndex = position239, tokenIndex239
							if buffer[position] != rune(' ') {
								goto l238
							}
							position++
						}
					l239:
						goto l220
					l238:
						position, tokenIndex = position238, tokenIndex238
					}
					if !matchDot() {
						goto l220
					}
				l236:
					{
						position237, tokenIndex237 := position, tokenIndex
						{
							position244, tokenIndex244 := position, tokenIndex
							{
								position245, tokenIndex245 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l246
								}
								position++
								goto l245
							l246:
								position, tokenIndex = position245, tokenIndex245
								if buffer[position] != rune('\r') {
									goto l247
								}
								position++
								goto l245
							l247:
								position, tokenIndex = position245, tokenIndex245
								if buffer[position] != rune('\n') {
									goto l248
								}
								position++
								goto l245
							l248:
								position, tokenIndex = position245, tokenIndex245
								if buffer[position] != rune('/') {
									goto l249
								}
								position++
								goto l245
							l249:
								position, tokenIndex = position245, tokenIndex245
								if buffer[position] != rune(' ') {
									goto l244
								}
								position++
							}
						l245:
							goto l237
						l244:
							position, tokenIndex = position244, tokenIndex244
						}
						if !matchDot() {
							goto l237
						}
						goto l236
					l237:
						position, tokenIndex = position237, tokenIndex237
					}
				}
			l222:
				add(ruletable_name, position221)
			}
			return true
		l220:
			position, tokenIndex = position220, tokenIndex220
			return false
		},
		/* 23 real_column_name <- <([a-z] / [A-Z] / [0-9] / '_')+> */
		func() bool {
			position250, tokenIndex250 := position, tokenIndex
			{
				position251 := position
				{
					position254, tokenIndex254 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l255
					}
					position++
					goto l254
				l255:
					position, tokenIndex = position254, tokenIndex254
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l256
					}
					position++
					goto l254
				l256:
					position, tokenIndex = position254, tokenIndex254
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l257
					}
					position++
					goto l254
				l257:
					position, tokenIndex = position254, tokenIndex254
					if buffer[position] != rune('_') {
						goto l250
					}
					position++
				}
			l254:
			l252:
				{
					position253, tokenIndex253 := position, tokenIndex
					{
						position258, tokenIndex258 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l259
						}
						position++
						goto l258
					l259:
						position, tokenIndex = position258, tokenIndex258
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l260
						}
						position++
						goto l258
					l260:
						position, tokenIndex = position258, tokenIndex258
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l261
						}
						position++
						goto l258
					l261:
						position, tokenIndex = position258, tokenIndex258
						if buffer[position] != rune('_') {
							goto l253
						}
						position++
					}
				l258:
					goto l252
				l253:
					position, tokenIndex = position253, tokenIndex253
				}
				add(rulereal_column_name, position251)
			}
			return true
		l250:
			position, tokenIndex = position250, tokenIndex250
			return false
		},
		/* 24 column_name <- <(('"' (!('\t' / '\r' / '\n' / '"') .)+ '"') / (!('\t' / '\r' / '\n' / '/' / ' ') .)+)> */
		func() bool {
			position262, tokenIndex262 := position, tokenIndex
			{
				position263 := position
				{
					position264, tokenIndex264 := position, tokenIndex
					if buffer[position] != rune('"') {
						goto l265
					}
					position++
					{
						position268, tokenIndex268 := position, tokenIndex
						{
							position269, tokenIndex269 := position, tokenIndex
							if buffer[position] != rune('\t') {
								goto l270
							}
							position++
							goto l269
						l270:
							position, tokenIndex = position269, tokenIndex269
							if buffer[position] != rune('\r') {
								goto l271
							}
							position++
							goto l269
						l271:
							position, tokenIndex = position269, tokenIndex269
							if buffer[position] != rune('\n') {
								goto l272
							}
							position++
							goto l269
						l272:
							position, tokenIndex = position269, tokenIndex269
							if buffer[position] != rune('"') {
								goto l268
							}
							position++
						}
					l269:
						goto l265
					l268:
						position, tokenIndex = position268, tokenIndex268
					}
					if !matchDot() {
						goto l265
					}
				l266:
					{
						position267, tokenIndex267 := position, tokenIndex
						{
							position273, tokenIndex273 := position, tokenIndex
							{
								position274, tokenIndex274 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l275
								}
								position++
								goto l274
							l275:
								position, tokenIndex = position274, tokenIndex274
								if buffer[position] != rune('\r') {
									goto l276
								}
								position++
								goto l274
							l276:
								position, tokenIndex = position274, tokenIndex274
								if buffer[position] != rune('\n') {
									goto l277
								}
								position++
								goto l274
							l277:
								position, tokenIndex = position274, tokenIndex274
								if buffer[position] != rune('"') {
									goto l273
								}
								position++
							}
						l274:
							goto l267
						l273:
							position, tokenIndex = position273, tokenIndex273
						}
						if !matchDot() {
							goto l267
						}
						goto l266
					l267:
						position, tokenIndex = position267, tokenIndex267
					}
					if buffer[position] != rune('"') {
						goto l265
					}
					position++
					goto l264
				l265:
					position, tokenIndex = position264, tokenIndex264
					{
						position280, tokenIndex280 := position, tokenIndex
						{
							position281, tokenIndex281 := position, tokenIndex
							if buffer[position] != rune('\t') {
								goto l282
							}
							position++
							goto l281
						l282:
							position, tokenIndex = position281, tokenIndex281
							if buffer[position] != rune('\r') {
								goto l283
							}
							position++
							goto l281
						l283:
							position, tokenIndex = position281, tokenIndex281
							if buffer[position] != rune('\n') {
								goto l284
							}
							position++
							goto l281
						l284:
							position, tokenIndex = position281, tokenIndex281
							if buffer[position] != rune('/') {
								goto l285
							}
							position++
							goto l281
						l285:
							position, tokenIndex = position281, tokenIndex281
							if buffer[position] != rune(' ') {
								goto l280
							}
							position++
						}
					l281:
						goto l262
					l280:
						position, tokenIndex = position280, tokenIndex280
					}
					if !matchDot() {
						goto l262
					}
				l278:
					{
						position279, tokenIndex279 := position, tokenIndex
						{
							position286, tokenIndex286 := position, tokenIndex
							{
								position287, tokenIndex287 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l288
								}
								position++
								goto l287
							l288:
								position, tokenIndex = position287, tokenIndex287
								if buffer[position] != rune('\r') {
									goto l289
								}
								position++
								goto l287
							l289:
								position, tokenIndex = position287, tokenIndex287
								if buffer[position] != rune('\n') {
									goto l290
								}
								position++
								goto l287
							l290:
								position, tokenIndex = position287, tokenIndex287
								if buffer[position] != rune('/') {
									goto l291
								}
								position++
								goto l287
							l291:
								position, tokenIndex = position287, tokenIndex287
								if buffer[position] != rune(' ') {
									goto l286
								}
								position++
							}
						l287:
							goto l279
						l286:
							position, tokenIndex = position286, tokenIndex286
						}
						if !matchDot() {
							goto l279
						}
						goto l278
					l279:
						position, tokenIndex = position279, tokenIndex279
					}
				}
			l264:
				add(rulecolumn_name, position263)
			}
			return true
		l262:
			position, tokenIndex = position262, tokenIndex262
			return false
		},
		/* 25 relation_point <- <([a-z] / [A-Z] / [0-9] / '_' / '.')+> */
		func() bool {
			position292, tokenIndex292 := position, tokenIndex
			{
				position293 := position
				{
					position296, tokenIndex296 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l297
					}
					position++
					goto l296
				l297:
					position, tokenIndex = position296, tokenIndex296
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l298
					}
					position++
					goto l296
				l298:
					position, tokenIndex = position296, tokenIndex296
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l299
					}
					position++
					goto l296
				l299:
					position, tokenIndex = position296, tokenIndex296
					if buffer[position] != rune('_') {
						goto l300
					}
					position++
					goto l296
				l300:
					position, tokenIndex = position296, tokenIndex296
					if buffer[position] != rune('.') {
						goto l292
					}
					position++
				}
			l296:
			l294:
				{
					position295, tokenIndex295 := position, tokenIndex
					{
						position301, tokenIndex301 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l302
						}
						position++
						goto l301
					l302:
						position, tokenIndex = position301, tokenIndex301
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l303
						}
						position++
						goto l301
					l303:
						position, tokenIndex = position301, tokenIndex301
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l304
						}
						position++
						goto l301
					l304:
						position, tokenIndex = position301, tokenIndex301
						if buffer[position] != rune('_') {
							goto l305
						}
						position++
						goto l301
					l305:
						position, tokenIndex = position301, tokenIndex301
						if buffer[position] != rune('.') {
							goto l295
						}
						position++
					}
				l301:
					goto l294
				l295:
					position, tokenIndex = position295, tokenIndex295
				}
				add(rulerelation_point, position293)
			}
			return true
		l292:
			position, tokenIndex = position292, tokenIndex292
			return false
		},
		/* 26 pkey <- <('+' / '*')> */
		func() bool {
			position306, tokenIndex306 := position, tokenIndex
			{
				position307 := position
				{
					position308, tokenIndex308 := position, tokenIndex
					if buffer[position] != rune('+') {
						goto l309
					}
					position++
					goto l308
				l309:
					position, tokenIndex = position308, tokenIndex308
					if buffer[position] != rune('*') {
						goto l306
					}
					position++
				}
			l308:
				add(rulepkey, position307)
			}
			return true
		l306:
			position, tokenIndex = position306, tokenIndex306
			return false
		},
		/* 27 col_type <- <([a-z] / [A-Z] / [0-9] / '_' / '(' / ')' / ' ' / '.' / ',')+> */
		func() bool {
			position310, tokenIndex310 := position, tokenIndex
			{
				position311 := position
				{
					position314, tokenIndex314 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l315
					}
					position++
					goto l314
				l315:
					position, tokenIndex = position314, tokenIndex314
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l316
					}
					position++
					goto l314
				l316:
					position, tokenIndex = position314, tokenIndex314
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l317
					}
					position++
					goto l314
				l317:
					position, tokenIndex = position314, tokenIndex314
					if buffer[position] != rune('_') {
						goto l318
					}
					position++
					goto l314
				l318:
					position, tokenIndex = position314, tokenIndex314
					if buffer[position] != rune('(') {
						goto l319
					}
					position++
					goto l314
				l319:
					position, tokenIndex = position314, tokenIndex314
					if buffer[position] != rune(')') {
						goto l320
					}
					position++
					goto l314
				l320:
					position, tokenIndex = position314, tokenIndex314
					if buffer[position] != rune(' ') {
						goto l321
					}
					position++
					goto l314
				l321:
					position, tokenIndex = position314, tokenIndex314
					if buffer[position] != rune('.') {
						goto l322
					}
					position++
					goto l314
				l322:
					position, tokenIndex = position314, tokenIndex314
					if buffer[position] != rune(',') {
						goto l310
					}
					position++
				}
			l314:
			l312:
				{
					position313, tokenIndex313 := position, tokenIndex
					{
						position323, tokenIndex323 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l324
						}
						position++
						goto l323
					l324:
						position, tokenIndex = position323, tokenIndex323
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l325
						}
						position++
						goto l323
					l325:
						position, tokenIndex = position323, tokenIndex323
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l326
						}
						position++
						goto l323
					l326:
						position, tokenIndex = position323, tokenIndex323
						if buffer[position] != rune('_') {
							goto l327
						}
						position++
						goto l323
					l327:
						position, tokenIndex = position323, tokenIndex323
						if buffer[position] != rune('(') {
							goto l328
						}
						position++
						goto l323
					l328:
						position, tokenIndex = position323, tokenIndex323
						if buffer[position] != rune(')') {
							goto l329
						}
						position++
						goto l323
					l329:
						position, tokenIndex = position323, tokenIndex323
						if buffer[position] != rune(' ') {
							goto l330
						}
						position++
						goto l323
					l330:
						position, tokenIndex = position323, tokenIndex323
						if buffer[position] != rune('.') {
							goto l331
						}
						position++
						goto l323
					l331:
						position, tokenIndex = position323, tokenIndex323
						if buffer[position] != rune(',') {
							goto l313
						}
						position++
					}
				l323:
					goto l312
				l313:
					position, tokenIndex = position313, tokenIndex313
				}
				add(rulecol_type, position311)
			}
			return true
		l310:
			position, tokenIndex = position310, tokenIndex310
			return false
		},
		/* 28 default <- <((!('\r' / '\n' / ']') .) / ('\\' ']'))*> */
		func() bool {
			{
				position333 := position
			l334:
				{
					position335, tokenIndex335 := position, tokenIndex
					{
						position336, tokenIndex336 := position, tokenIndex
						{
							position338, tokenIndex338 := position, tokenIndex
							{
								position339, tokenIndex339 := position, tokenIndex
								if buffer[position] != rune('\r') {
									goto l340
								}
								position++
								goto l339
							l340:
								position, tokenIndex = position339, tokenIndex339
								if buffer[position] != rune('\n') {
									goto l341
								}
								position++
								goto l339
							l341:
								position, tokenIndex = position339, tokenIndex339
								if buffer[position] != rune(']') {
									goto l338
								}
								position++
							}
						l339:
							goto l337
						l338:
							position, tokenIndex = position338, tokenIndex338
						}
						if !matchDot() {
							goto l337
						}
						goto l336
					l337:
						position, tokenIndex = position336, tokenIndex336
						if buffer[position] != rune('\\') {
							goto l335
						}
						position++
						if buffer[position] != rune(']') {
							goto l335
						}
						position++
					}
				l336:
					goto l334
				l335:
					position, tokenIndex = position335, tokenIndex335
				}
				add(ruledefault, position333)
			}
			return true
		},
		/* 29 cardinality_right <- <cardinality> */
		func() bool {
			position342, tokenIndex342 := position, tokenIndex
			{
				position343 := position
				if !_rules[rulecardinality]() {
					goto l342
				}
				add(rulecardinality_right, position343)
			}
			return true
		l342:
			position, tokenIndex = position342, tokenIndex342
			return false
		},
		/* 30 cardinality_left <- <cardinality> */
		func() bool {
			position344, tokenIndex344 := position, tokenIndex
			{
				position345 := position
				if !_rules[rulecardinality]() {
					goto l344
				}
				add(rulecardinality_left, position345)
			}
			return true
		l344:
			position, tokenIndex = position344, tokenIndex344
			return false
		},
		/* 31 cardinality <- <(('0' / '1' / '*') (. . ('0' / '1' / '*'))?)> */
		func() bool {
			position346, tokenIndex346 := position, tokenIndex
			{
				position347 := position
				{
					position348, tokenIndex348 := position, tokenIndex
					if buffer[position] != rune('0') {
						goto l349
					}
					position++
					goto l348
				l349:
					position, tokenIndex = position348, tokenIndex348
					if buffer[position] != rune('1') {
						goto l350
					}
					position++
					goto l348
				l350:
					position, tokenIndex = position348, tokenIndex348
					if buffer[position] != rune('*') {
						goto l346
					}
					position++
				}
			l348:
				{
					position351, tokenIndex351 := position, tokenIndex
					if !matchDot() {
						goto l351
					}
					if !matchDot() {
						goto l351
					}
					{
						position353, tokenIndex353 := position, tokenIndex
						if buffer[position] != rune('0') {
							goto l354
						}
						position++
						goto l353
					l354:
						position, tokenIndex = position353, tokenIndex353
						if buffer[position] != rune('1') {
							goto l355
						}
						position++
						goto l353
					l355:
						position, tokenIndex = position353, tokenIndex353
						if buffer[position] != rune('*') {
							goto l351
						}
						position++
					}
				l353:
					goto l352
				l351:
					position, tokenIndex = position351, tokenIndex351
				}
			l352:
				add(rulecardinality, position347)
			}
			return true
		l346:
			position, tokenIndex = position346, tokenIndex346
			return false
		},
		nil,
		/* 34 Action0 <- <{p.Err(begin, buffer)}> */
		func() bool {
			{
				add(ruleAction0, position)
			}
			return true
		},
		/* 35 Action1 <- <{p.Err(begin, buffer)}> */
		func() bool {
			{
				add(ruleAction1, position)
			}
			return true
		},
		/* 36 Action2 <- <{p.setTitle(text)}> */
		func() bool {
			{
				add(ruleAction2, position)
			}
			return true
		},
		/* 37 Action3 <- <{p.addTableTitleReal(text)}> */
		func() bool {
			{
				add(ruleAction3, position)
			}
			return true
		},
		/* 38 Action4 <- <{p.addTableTitle(text)}> */
		func() bool {
			{
				add(ruleAction4, position)
			}
			return true
		},
		/* 39 Action5 <- <{ p.addPrimaryKey(text) }> */
		func() bool {
			{
				add(ruleAction5, position)
			}
			return true
		},
		/* 40 Action6 <- <{ p.setColumnNameReal(text) }> */
		func() bool {
			{
				add(ruleAction6, position)
			}
			return true
		},
		/* 41 Action7 <- <{ p.setColumnName(text) }> */
		func() bool {
			{
				add(ruleAction7, position)
			}
			return true
		},
		/* 42 Action8 <- <{ p.addColumnType(text) }> */
		func() bool {
			{
				add(ruleAction8, position)
			}
			return true
		},
		/* 43 Action9 <- <{ p.setNotNull() }> */
		func() bool {
			{
				add(ruleAction9, position)
			}
			return true
		},
		/* 44 Action10 <- <{ p.setUnique() }> */
		func() bool {
			{
				add(ruleAction10, position)
			}
			return true
		},
		/* 45 Action11 <- <{ p.setColumnDefault(text) }> */
		func() bool {
			{
				add(ruleAction11, position)
			}
			return true
		},
		/* 46 Action12 <- <{ p.setWithoutErd() }> */
		func() bool {
			{
				add(ruleAction12, position)
			}
			return true
		},
		/* 47 Action13 <- <{ p.setRelationSource(text) }> */
		func() bool {
			{
				add(ruleAction13, position)
			}
			return true
		},
		/* 48 Action14 <- <{ p.setRelationDestination(text) }> */
		func() bool {
			{
				add(ruleAction14, position)
			}
			return true
		},
		/* 49 Action15 <- <{ p.setRelationTableNameReal(text) }> */
		func() bool {
			{
				add(ruleAction15, position)
			}
			return true
		},
		/* 50 Action16 <- <{ p.addComment(text) }> */
		func() bool {
			{
				add(ruleAction16, position)
			}
			return true
		},
		/* 51 Action17 <- <{p.setIndexName(text)}> */
		func() bool {
			{
				add(ruleAction17, position)
			}
			return true
		},
		/* 52 Action18 <- <{p.setIndexColumn(text)}> */
		func() bool {
			{
				add(ruleAction18, position)
			}
			return true
		},
		/* 53 Action19 <- <{p.setIndexColumn(text)}> */
		func() bool {
			{
				add(ruleAction19, position)
			}
			return true
		},
		/* 54 Action20 <- <{ p.setUniqueIndex() }> */
		func() bool {
			{
				add(ruleAction20, position)
			}
			return true
		},
	}
	p.rules = _rules
}
