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
		/* 9 column_attribute <- <(space+ (<pkey> Action5)? <real_column_name> Action6 ('/' <column_name> Action7)? space+ '[' <col_type> Action8 ']' (('[' notnull Action9 ']') / ('[' unique Action10 ']'))* ('[' '=' <default> Action11 ']')? newline?)> */
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
							goto l85
						}
						position++
						if !_rules[ruleunique]() {
							goto l85
						}
						if !_rules[ruleAction10]() {
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
					position88, tokenIndex88 := position, tokenIndex
					if buffer[position] != rune('[') {
						goto l88
					}
					position++
					if buffer[position] != rune('=') {
						goto l88
					}
					position++
					{
						position90 := position
						if !_rules[ruledefault]() {
							goto l88
						}
						add(rulePegText, position90)
					}
					if !_rules[ruleAction11]() {
						goto l88
					}
					if buffer[position] != rune(']') {
						goto l88
					}
					position++
					goto l89
				l88:
					position, tokenIndex = position88, tokenIndex88
				}
			l89:
				{
					position91, tokenIndex91 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l91
					}
					goto l92
				l91:
					position, tokenIndex = position91, tokenIndex91
				}
			l92:
				add(rulecolumn_attribute, position71)
			}
			return true
		l70:
			position, tokenIndex = position70, tokenIndex70
			return false
		},
		/* 10 relation <- <((<cardinality_left> Action12)? space* ('-' '-') space* (<cardinality_right> Action13 space)? space* <relation_point> Action14)> */
		func() bool {
			position93, tokenIndex93 := position, tokenIndex
			{
				position94 := position
				{
					position95, tokenIndex95 := position, tokenIndex
					{
						position97 := position
						if !_rules[rulecardinality_left]() {
							goto l95
						}
						add(rulePegText, position97)
					}
					if !_rules[ruleAction12]() {
						goto l95
					}
					goto l96
				l95:
					position, tokenIndex = position95, tokenIndex95
				}
			l96:
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
				if buffer[position] != rune('-') {
					goto l93
				}
				position++
				if buffer[position] != rune('-') {
					goto l93
				}
				position++
			l100:
				{
					position101, tokenIndex101 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l101
					}
					goto l100
				l101:
					position, tokenIndex = position101, tokenIndex101
				}
				{
					position102, tokenIndex102 := position, tokenIndex
					{
						position104 := position
						if !_rules[rulecardinality_right]() {
							goto l102
						}
						add(rulePegText, position104)
					}
					if !_rules[ruleAction13]() {
						goto l102
					}
					if !_rules[rulespace]() {
						goto l102
					}
					goto l103
				l102:
					position, tokenIndex = position102, tokenIndex102
				}
			l103:
			l105:
				{
					position106, tokenIndex106 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l106
					}
					goto l105
				l106:
					position, tokenIndex = position106, tokenIndex106
				}
				{
					position107 := position
					if !_rules[rulerelation_point]() {
						goto l93
					}
					add(rulePegText, position107)
				}
				if !_rules[ruleAction14]() {
					goto l93
				}
				add(rulerelation, position94)
			}
			return true
		l93:
			position, tokenIndex = position93, tokenIndex93
			return false
		},
		/* 11 column_comment <- <(space+ '#' space? <comment_string> Action15)> */
		func() bool {
			position108, tokenIndex108 := position, tokenIndex
			{
				position109 := position
				if !_rules[rulespace]() {
					goto l108
				}
			l110:
				{
					position111, tokenIndex111 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l111
					}
					goto l110
				l111:
					position, tokenIndex = position111, tokenIndex111
				}
				if buffer[position] != rune('#') {
					goto l108
				}
				position++
				{
					position112, tokenIndex112 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l112
					}
					goto l113
				l112:
					position, tokenIndex = position112, tokenIndex112
				}
			l113:
				{
					position114 := position
					if !_rules[rulecomment_string]() {
						goto l108
					}
					add(rulePegText, position114)
				}
				if !_rules[ruleAction15]() {
					goto l108
				}
				add(rulecolumn_comment, position109)
			}
			return true
		l108:
			position, tokenIndex = position108, tokenIndex108
			return false
		},
		/* 12 index_info <- <(space+ (('i' / 'I') ('n' / 'N') ('d' / 'D') ('e' / 'E') ('x' / 'X')) space+ <real_column_name> Action16 space+ '(' space* <real_column_name> Action17 (space* ',' space* <real_column_name> Action18 space*)* space* ')' (space+ ('u' 'n' 'i' 'q' 'u' 'e') Action19)? space* newline*)> */
		func() bool {
			position115, tokenIndex115 := position, tokenIndex
			{
				position116 := position
				if !_rules[rulespace]() {
					goto l115
				}
			l117:
				{
					position118, tokenIndex118 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l118
					}
					goto l117
				l118:
					position, tokenIndex = position118, tokenIndex118
				}
				{
					position119, tokenIndex119 := position, tokenIndex
					if buffer[position] != rune('i') {
						goto l120
					}
					position++
					goto l119
				l120:
					position, tokenIndex = position119, tokenIndex119
					if buffer[position] != rune('I') {
						goto l115
					}
					position++
				}
			l119:
				{
					position121, tokenIndex121 := position, tokenIndex
					if buffer[position] != rune('n') {
						goto l122
					}
					position++
					goto l121
				l122:
					position, tokenIndex = position121, tokenIndex121
					if buffer[position] != rune('N') {
						goto l115
					}
					position++
				}
			l121:
				{
					position123, tokenIndex123 := position, tokenIndex
					if buffer[position] != rune('d') {
						goto l124
					}
					position++
					goto l123
				l124:
					position, tokenIndex = position123, tokenIndex123
					if buffer[position] != rune('D') {
						goto l115
					}
					position++
				}
			l123:
				{
					position125, tokenIndex125 := position, tokenIndex
					if buffer[position] != rune('e') {
						goto l126
					}
					position++
					goto l125
				l126:
					position, tokenIndex = position125, tokenIndex125
					if buffer[position] != rune('E') {
						goto l115
					}
					position++
				}
			l125:
				{
					position127, tokenIndex127 := position, tokenIndex
					if buffer[position] != rune('x') {
						goto l128
					}
					position++
					goto l127
				l128:
					position, tokenIndex = position127, tokenIndex127
					if buffer[position] != rune('X') {
						goto l115
					}
					position++
				}
			l127:
				if !_rules[rulespace]() {
					goto l115
				}
			l129:
				{
					position130, tokenIndex130 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l130
					}
					goto l129
				l130:
					position, tokenIndex = position130, tokenIndex130
				}
				{
					position131 := position
					if !_rules[rulereal_column_name]() {
						goto l115
					}
					add(rulePegText, position131)
				}
				if !_rules[ruleAction16]() {
					goto l115
				}
				if !_rules[rulespace]() {
					goto l115
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
				if buffer[position] != rune('(') {
					goto l115
				}
				position++
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
				{
					position136 := position
					if !_rules[rulereal_column_name]() {
						goto l115
					}
					add(rulePegText, position136)
				}
				if !_rules[ruleAction17]() {
					goto l115
				}
			l137:
				{
					position138, tokenIndex138 := position, tokenIndex
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
					if buffer[position] != rune(',') {
						goto l138
					}
					position++
				l141:
					{
						position142, tokenIndex142 := position, tokenIndex
						if !_rules[rulespace]() {
							goto l142
						}
						goto l141
					l142:
						position, tokenIndex = position142, tokenIndex142
					}
					{
						position143 := position
						if !_rules[rulereal_column_name]() {
							goto l138
						}
						add(rulePegText, position143)
					}
					if !_rules[ruleAction18]() {
						goto l138
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
					goto l137
				l138:
					position, tokenIndex = position138, tokenIndex138
				}
			l146:
				{
					position147, tokenIndex147 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l147
					}
					goto l146
				l147:
					position, tokenIndex = position147, tokenIndex147
				}
				if buffer[position] != rune(')') {
					goto l115
				}
				position++
				{
					position148, tokenIndex148 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l148
					}
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
					if buffer[position] != rune('u') {
						goto l148
					}
					position++
					if buffer[position] != rune('n') {
						goto l148
					}
					position++
					if buffer[position] != rune('i') {
						goto l148
					}
					position++
					if buffer[position] != rune('q') {
						goto l148
					}
					position++
					if buffer[position] != rune('u') {
						goto l148
					}
					position++
					if buffer[position] != rune('e') {
						goto l148
					}
					position++
					if !_rules[ruleAction19]() {
						goto l148
					}
					goto l149
				l148:
					position, tokenIndex = position148, tokenIndex148
				}
			l149:
			l152:
				{
					position153, tokenIndex153 := position, tokenIndex
					if !_rules[rulespace]() {
						goto l153
					}
					goto l152
				l153:
					position, tokenIndex = position153, tokenIndex153
				}
			l154:
				{
					position155, tokenIndex155 := position, tokenIndex
					if !_rules[rulenewline]() {
						goto l155
					}
					goto l154
				l155:
					position, tokenIndex = position155, tokenIndex155
				}
				add(ruleindex_info, position116)
			}
			return true
		l115:
			position, tokenIndex = position115, tokenIndex115
			return false
		},
		/* 13 title <- <(!('\r' / '\n') .)+> */
		func() bool {
			position156, tokenIndex156 := position, tokenIndex
			{
				position157 := position
				{
					position160, tokenIndex160 := position, tokenIndex
					{
						position161, tokenIndex161 := position, tokenIndex
						if buffer[position] != rune('\r') {
							goto l162
						}
						position++
						goto l161
					l162:
						position, tokenIndex = position161, tokenIndex161
						if buffer[position] != rune('\n') {
							goto l160
						}
						position++
					}
				l161:
					goto l156
				l160:
					position, tokenIndex = position160, tokenIndex160
				}
				if !matchDot() {
					goto l156
				}
			l158:
				{
					position159, tokenIndex159 := position, tokenIndex
					{
						position163, tokenIndex163 := position, tokenIndex
						{
							position164, tokenIndex164 := position, tokenIndex
							if buffer[position] != rune('\r') {
								goto l165
							}
							position++
							goto l164
						l165:
							position, tokenIndex = position164, tokenIndex164
							if buffer[position] != rune('\n') {
								goto l163
							}
							position++
						}
					l164:
						goto l159
					l163:
						position, tokenIndex = position163, tokenIndex163
					}
					if !matchDot() {
						goto l159
					}
					goto l158
				l159:
					position, tokenIndex = position159, tokenIndex159
				}
				add(ruletitle, position157)
			}
			return true
		l156:
			position, tokenIndex = position156, tokenIndex156
			return false
		},
		/* 14 comment_string <- <(!('\r' / '\n') .)*> */
		func() bool {
			{
				position167 := position
			l168:
				{
					position169, tokenIndex169 := position, tokenIndex
					{
						position170, tokenIndex170 := position, tokenIndex
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
								goto l170
							}
							position++
						}
					l171:
						goto l169
					l170:
						position, tokenIndex = position170, tokenIndex170
					}
					if !matchDot() {
						goto l169
					}
					goto l168
				l169:
					position, tokenIndex = position169, tokenIndex169
				}
				add(rulecomment_string, position167)
			}
			return true
		},
		/* 15 whitespace <- <(' ' / '\t' / '\r' / '\n')+> */
		func() bool {
			position173, tokenIndex173 := position, tokenIndex
			{
				position174 := position
				{
					position177, tokenIndex177 := position, tokenIndex
					if buffer[position] != rune(' ') {
						goto l178
					}
					position++
					goto l177
				l178:
					position, tokenIndex = position177, tokenIndex177
					if buffer[position] != rune('\t') {
						goto l179
					}
					position++
					goto l177
				l179:
					position, tokenIndex = position177, tokenIndex177
					if buffer[position] != rune('\r') {
						goto l180
					}
					position++
					goto l177
				l180:
					position, tokenIndex = position177, tokenIndex177
					if buffer[position] != rune('\n') {
						goto l173
					}
					position++
				}
			l177:
			l175:
				{
					position176, tokenIndex176 := position, tokenIndex
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
							goto l183
						}
						position++
						goto l181
					l183:
						position, tokenIndex = position181, tokenIndex181
						if buffer[position] != rune('\r') {
							goto l184
						}
						position++
						goto l181
					l184:
						position, tokenIndex = position181, tokenIndex181
						if buffer[position] != rune('\n') {
							goto l176
						}
						position++
					}
				l181:
					goto l175
				l176:
					position, tokenIndex = position176, tokenIndex176
				}
				add(rulewhitespace, position174)
			}
			return true
		l173:
			position, tokenIndex = position173, tokenIndex173
			return false
		},
		/* 16 newline <- <('\r' / '\n')+> */
		func() bool {
			position185, tokenIndex185 := position, tokenIndex
			{
				position186 := position
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
						goto l185
					}
					position++
				}
			l189:
			l187:
				{
					position188, tokenIndex188 := position, tokenIndex
					{
						position191, tokenIndex191 := position, tokenIndex
						if buffer[position] != rune('\r') {
							goto l192
						}
						position++
						goto l191
					l192:
						position, tokenIndex = position191, tokenIndex191
						if buffer[position] != rune('\n') {
							goto l188
						}
						position++
					}
				l191:
					goto l187
				l188:
					position, tokenIndex = position188, tokenIndex188
				}
				add(rulenewline, position186)
			}
			return true
		l185:
			position, tokenIndex = position185, tokenIndex185
			return false
		},
		/* 17 space <- <(' ' / '\t')+> */
		func() bool {
			position193, tokenIndex193 := position, tokenIndex
			{
				position194 := position
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
						goto l193
					}
					position++
				}
			l197:
			l195:
				{
					position196, tokenIndex196 := position, tokenIndex
					{
						position199, tokenIndex199 := position, tokenIndex
						if buffer[position] != rune(' ') {
							goto l200
						}
						position++
						goto l199
					l200:
						position, tokenIndex = position199, tokenIndex199
						if buffer[position] != rune('\t') {
							goto l196
						}
						position++
					}
				l199:
					goto l195
				l196:
					position, tokenIndex = position196, tokenIndex196
				}
				add(rulespace, position194)
			}
			return true
		l193:
			position, tokenIndex = position193, tokenIndex193
			return false
		},
		/* 18 notnull <- <('N' 'N')> */
		func() bool {
			position201, tokenIndex201 := position, tokenIndex
			{
				position202 := position
				if buffer[position] != rune('N') {
					goto l201
				}
				position++
				if buffer[position] != rune('N') {
					goto l201
				}
				position++
				add(rulenotnull, position202)
			}
			return true
		l201:
			position, tokenIndex = position201, tokenIndex201
			return false
		},
		/* 19 unique <- <'U'> */
		func() bool {
			position203, tokenIndex203 := position, tokenIndex
			{
				position204 := position
				if buffer[position] != rune('U') {
					goto l203
				}
				position++
				add(ruleunique, position204)
			}
			return true
		l203:
			position, tokenIndex = position203, tokenIndex203
			return false
		},
		/* 20 real_table_name <- <([a-z] / [A-Z] / [0-9] / '_')+> */
		func() bool {
			position205, tokenIndex205 := position, tokenIndex
			{
				position206 := position
				{
					position209, tokenIndex209 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l210
					}
					position++
					goto l209
				l210:
					position, tokenIndex = position209, tokenIndex209
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l211
					}
					position++
					goto l209
				l211:
					position, tokenIndex = position209, tokenIndex209
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l212
					}
					position++
					goto l209
				l212:
					position, tokenIndex = position209, tokenIndex209
					if buffer[position] != rune('_') {
						goto l205
					}
					position++
				}
			l209:
			l207:
				{
					position208, tokenIndex208 := position, tokenIndex
					{
						position213, tokenIndex213 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l214
						}
						position++
						goto l213
					l214:
						position, tokenIndex = position213, tokenIndex213
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l215
						}
						position++
						goto l213
					l215:
						position, tokenIndex = position213, tokenIndex213
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l216
						}
						position++
						goto l213
					l216:
						position, tokenIndex = position213, tokenIndex213
						if buffer[position] != rune('_') {
							goto l208
						}
						position++
					}
				l213:
					goto l207
				l208:
					position, tokenIndex = position208, tokenIndex208
				}
				add(rulereal_table_name, position206)
			}
			return true
		l205:
			position, tokenIndex = position205, tokenIndex205
			return false
		},
		/* 21 table_name <- <(('"' (!('\t' / '\r' / '\n' / '"') .)+ '"') / (!('\t' / '\r' / '\n' / '/' / ' ') .)+)> */
		func() bool {
			position217, tokenIndex217 := position, tokenIndex
			{
				position218 := position
				{
					position219, tokenIndex219 := position, tokenIndex
					if buffer[position] != rune('"') {
						goto l220
					}
					position++
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
							if buffer[position] != rune('"') {
								goto l223
							}
							position++
						}
					l224:
						goto l220
					l223:
						position, tokenIndex = position223, tokenIndex223
					}
					if !matchDot() {
						goto l220
					}
				l221:
					{
						position222, tokenIndex222 := position, tokenIndex
						{
							position228, tokenIndex228 := position, tokenIndex
							{
								position229, tokenIndex229 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l230
								}
								position++
								goto l229
							l230:
								position, tokenIndex = position229, tokenIndex229
								if buffer[position] != rune('\r') {
									goto l231
								}
								position++
								goto l229
							l231:
								position, tokenIndex = position229, tokenIndex229
								if buffer[position] != rune('\n') {
									goto l232
								}
								position++
								goto l229
							l232:
								position, tokenIndex = position229, tokenIndex229
								if buffer[position] != rune('"') {
									goto l228
								}
								position++
							}
						l229:
							goto l222
						l228:
							position, tokenIndex = position228, tokenIndex228
						}
						if !matchDot() {
							goto l222
						}
						goto l221
					l222:
						position, tokenIndex = position222, tokenIndex222
					}
					if buffer[position] != rune('"') {
						goto l220
					}
					position++
					goto l219
				l220:
					position, tokenIndex = position219, tokenIndex219
					{
						position235, tokenIndex235 := position, tokenIndex
						{
							position236, tokenIndex236 := position, tokenIndex
							if buffer[position] != rune('\t') {
								goto l237
							}
							position++
							goto l236
						l237:
							position, tokenIndex = position236, tokenIndex236
							if buffer[position] != rune('\r') {
								goto l238
							}
							position++
							goto l236
						l238:
							position, tokenIndex = position236, tokenIndex236
							if buffer[position] != rune('\n') {
								goto l239
							}
							position++
							goto l236
						l239:
							position, tokenIndex = position236, tokenIndex236
							if buffer[position] != rune('/') {
								goto l240
							}
							position++
							goto l236
						l240:
							position, tokenIndex = position236, tokenIndex236
							if buffer[position] != rune(' ') {
								goto l235
							}
							position++
						}
					l236:
						goto l217
					l235:
						position, tokenIndex = position235, tokenIndex235
					}
					if !matchDot() {
						goto l217
					}
				l233:
					{
						position234, tokenIndex234 := position, tokenIndex
						{
							position241, tokenIndex241 := position, tokenIndex
							{
								position242, tokenIndex242 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l243
								}
								position++
								goto l242
							l243:
								position, tokenIndex = position242, tokenIndex242
								if buffer[position] != rune('\r') {
									goto l244
								}
								position++
								goto l242
							l244:
								position, tokenIndex = position242, tokenIndex242
								if buffer[position] != rune('\n') {
									goto l245
								}
								position++
								goto l242
							l245:
								position, tokenIndex = position242, tokenIndex242
								if buffer[position] != rune('/') {
									goto l246
								}
								position++
								goto l242
							l246:
								position, tokenIndex = position242, tokenIndex242
								if buffer[position] != rune(' ') {
									goto l241
								}
								position++
							}
						l242:
							goto l234
						l241:
							position, tokenIndex = position241, tokenIndex241
						}
						if !matchDot() {
							goto l234
						}
						goto l233
					l234:
						position, tokenIndex = position234, tokenIndex234
					}
				}
			l219:
				add(ruletable_name, position218)
			}
			return true
		l217:
			position, tokenIndex = position217, tokenIndex217
			return false
		},
		/* 22 real_column_name <- <([a-z] / [A-Z] / [0-9] / '_')+> */
		func() bool {
			position247, tokenIndex247 := position, tokenIndex
			{
				position248 := position
				{
					position251, tokenIndex251 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l252
					}
					position++
					goto l251
				l252:
					position, tokenIndex = position251, tokenIndex251
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l253
					}
					position++
					goto l251
				l253:
					position, tokenIndex = position251, tokenIndex251
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l254
					}
					position++
					goto l251
				l254:
					position, tokenIndex = position251, tokenIndex251
					if buffer[position] != rune('_') {
						goto l247
					}
					position++
				}
			l251:
			l249:
				{
					position250, tokenIndex250 := position, tokenIndex
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
							goto l250
						}
						position++
					}
				l255:
					goto l249
				l250:
					position, tokenIndex = position250, tokenIndex250
				}
				add(rulereal_column_name, position248)
			}
			return true
		l247:
			position, tokenIndex = position247, tokenIndex247
			return false
		},
		/* 23 column_name <- <(('"' (!('\t' / '\r' / '\n' / '"') .)+ '"') / (!('\t' / '\r' / '\n' / '/' / ' ') .)+)> */
		func() bool {
			position259, tokenIndex259 := position, tokenIndex
			{
				position260 := position
				{
					position261, tokenIndex261 := position, tokenIndex
					if buffer[position] != rune('"') {
						goto l262
					}
					position++
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
							if buffer[position] != rune('"') {
								goto l265
							}
							position++
						}
					l266:
						goto l262
					l265:
						position, tokenIndex = position265, tokenIndex265
					}
					if !matchDot() {
						goto l262
					}
				l263:
					{
						position264, tokenIndex264 := position, tokenIndex
						{
							position270, tokenIndex270 := position, tokenIndex
							{
								position271, tokenIndex271 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l272
								}
								position++
								goto l271
							l272:
								position, tokenIndex = position271, tokenIndex271
								if buffer[position] != rune('\r') {
									goto l273
								}
								position++
								goto l271
							l273:
								position, tokenIndex = position271, tokenIndex271
								if buffer[position] != rune('\n') {
									goto l274
								}
								position++
								goto l271
							l274:
								position, tokenIndex = position271, tokenIndex271
								if buffer[position] != rune('"') {
									goto l270
								}
								position++
							}
						l271:
							goto l264
						l270:
							position, tokenIndex = position270, tokenIndex270
						}
						if !matchDot() {
							goto l264
						}
						goto l263
					l264:
						position, tokenIndex = position264, tokenIndex264
					}
					if buffer[position] != rune('"') {
						goto l262
					}
					position++
					goto l261
				l262:
					position, tokenIndex = position261, tokenIndex261
					{
						position277, tokenIndex277 := position, tokenIndex
						{
							position278, tokenIndex278 := position, tokenIndex
							if buffer[position] != rune('\t') {
								goto l279
							}
							position++
							goto l278
						l279:
							position, tokenIndex = position278, tokenIndex278
							if buffer[position] != rune('\r') {
								goto l280
							}
							position++
							goto l278
						l280:
							position, tokenIndex = position278, tokenIndex278
							if buffer[position] != rune('\n') {
								goto l281
							}
							position++
							goto l278
						l281:
							position, tokenIndex = position278, tokenIndex278
							if buffer[position] != rune('/') {
								goto l282
							}
							position++
							goto l278
						l282:
							position, tokenIndex = position278, tokenIndex278
							if buffer[position] != rune(' ') {
								goto l277
							}
							position++
						}
					l278:
						goto l259
					l277:
						position, tokenIndex = position277, tokenIndex277
					}
					if !matchDot() {
						goto l259
					}
				l275:
					{
						position276, tokenIndex276 := position, tokenIndex
						{
							position283, tokenIndex283 := position, tokenIndex
							{
								position284, tokenIndex284 := position, tokenIndex
								if buffer[position] != rune('\t') {
									goto l285
								}
								position++
								goto l284
							l285:
								position, tokenIndex = position284, tokenIndex284
								if buffer[position] != rune('\r') {
									goto l286
								}
								position++
								goto l284
							l286:
								position, tokenIndex = position284, tokenIndex284
								if buffer[position] != rune('\n') {
									goto l287
								}
								position++
								goto l284
							l287:
								position, tokenIndex = position284, tokenIndex284
								if buffer[position] != rune('/') {
									goto l288
								}
								position++
								goto l284
							l288:
								position, tokenIndex = position284, tokenIndex284
								if buffer[position] != rune(' ') {
									goto l283
								}
								position++
							}
						l284:
							goto l276
						l283:
							position, tokenIndex = position283, tokenIndex283
						}
						if !matchDot() {
							goto l276
						}
						goto l275
					l276:
						position, tokenIndex = position276, tokenIndex276
					}
				}
			l261:
				add(rulecolumn_name, position260)
			}
			return true
		l259:
			position, tokenIndex = position259, tokenIndex259
			return false
		},
		/* 24 relation_point <- <([a-z] / [A-Z] / [0-9] / '_' / '.')+> */
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
					if buffer[position] != rune('.') {
						goto l289
					}
					position++
				}
			l293:
			l291:
				{
					position292, tokenIndex292 := position, tokenIndex
					{
						position298, tokenIndex298 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l299
						}
						position++
						goto l298
					l299:
						position, tokenIndex = position298, tokenIndex298
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l300
						}
						position++
						goto l298
					l300:
						position, tokenIndex = position298, tokenIndex298
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l301
						}
						position++
						goto l298
					l301:
						position, tokenIndex = position298, tokenIndex298
						if buffer[position] != rune('_') {
							goto l302
						}
						position++
						goto l298
					l302:
						position, tokenIndex = position298, tokenIndex298
						if buffer[position] != rune('.') {
							goto l292
						}
						position++
					}
				l298:
					goto l291
				l292:
					position, tokenIndex = position292, tokenIndex292
				}
				add(rulerelation_point, position290)
			}
			return true
		l289:
			position, tokenIndex = position289, tokenIndex289
			return false
		},
		/* 25 pkey <- <('+' / '*')> */
		func() bool {
			position303, tokenIndex303 := position, tokenIndex
			{
				position304 := position
				{
					position305, tokenIndex305 := position, tokenIndex
					if buffer[position] != rune('+') {
						goto l306
					}
					position++
					goto l305
				l306:
					position, tokenIndex = position305, tokenIndex305
					if buffer[position] != rune('*') {
						goto l303
					}
					position++
				}
			l305:
				add(rulepkey, position304)
			}
			return true
		l303:
			position, tokenIndex = position303, tokenIndex303
			return false
		},
		/* 26 col_type <- <([a-z] / [A-Z] / [0-9] / '_' / '(' / ')' / ' ' / '.' / ',')+> */
		func() bool {
			position307, tokenIndex307 := position, tokenIndex
			{
				position308 := position
				{
					position311, tokenIndex311 := position, tokenIndex
					if c := buffer[position]; c < rune('a') || c > rune('z') {
						goto l312
					}
					position++
					goto l311
				l312:
					position, tokenIndex = position311, tokenIndex311
					if c := buffer[position]; c < rune('A') || c > rune('Z') {
						goto l313
					}
					position++
					goto l311
				l313:
					position, tokenIndex = position311, tokenIndex311
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l314
					}
					position++
					goto l311
				l314:
					position, tokenIndex = position311, tokenIndex311
					if buffer[position] != rune('_') {
						goto l315
					}
					position++
					goto l311
				l315:
					position, tokenIndex = position311, tokenIndex311
					if buffer[position] != rune('(') {
						goto l316
					}
					position++
					goto l311
				l316:
					position, tokenIndex = position311, tokenIndex311
					if buffer[position] != rune(')') {
						goto l317
					}
					position++
					goto l311
				l317:
					position, tokenIndex = position311, tokenIndex311
					if buffer[position] != rune(' ') {
						goto l318
					}
					position++
					goto l311
				l318:
					position, tokenIndex = position311, tokenIndex311
					if buffer[position] != rune('.') {
						goto l319
					}
					position++
					goto l311
				l319:
					position, tokenIndex = position311, tokenIndex311
					if buffer[position] != rune(',') {
						goto l307
					}
					position++
				}
			l311:
			l309:
				{
					position310, tokenIndex310 := position, tokenIndex
					{
						position320, tokenIndex320 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l321
						}
						position++
						goto l320
					l321:
						position, tokenIndex = position320, tokenIndex320
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l322
						}
						position++
						goto l320
					l322:
						position, tokenIndex = position320, tokenIndex320
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l323
						}
						position++
						goto l320
					l323:
						position, tokenIndex = position320, tokenIndex320
						if buffer[position] != rune('_') {
							goto l324
						}
						position++
						goto l320
					l324:
						position, tokenIndex = position320, tokenIndex320
						if buffer[position] != rune('(') {
							goto l325
						}
						position++
						goto l320
					l325:
						position, tokenIndex = position320, tokenIndex320
						if buffer[position] != rune(')') {
							goto l326
						}
						position++
						goto l320
					l326:
						position, tokenIndex = position320, tokenIndex320
						if buffer[position] != rune(' ') {
							goto l327
						}
						position++
						goto l320
					l327:
						position, tokenIndex = position320, tokenIndex320
						if buffer[position] != rune('.') {
							goto l328
						}
						position++
						goto l320
					l328:
						position, tokenIndex = position320, tokenIndex320
						if buffer[position] != rune(',') {
							goto l310
						}
						position++
					}
				l320:
					goto l309
				l310:
					position, tokenIndex = position310, tokenIndex310
				}
				add(rulecol_type, position308)
			}
			return true
		l307:
			position, tokenIndex = position307, tokenIndex307
			return false
		},
		/* 27 default <- <((!('\r' / '\n' / ']') .) / ('\\' ']'))*> */
		func() bool {
			{
				position330 := position
			l331:
				{
					position332, tokenIndex332 := position, tokenIndex
					{
						position333, tokenIndex333 := position, tokenIndex
						{
							position335, tokenIndex335 := position, tokenIndex
							{
								position336, tokenIndex336 := position, tokenIndex
								if buffer[position] != rune('\r') {
									goto l337
								}
								position++
								goto l336
							l337:
								position, tokenIndex = position336, tokenIndex336
								if buffer[position] != rune('\n') {
									goto l338
								}
								position++
								goto l336
							l338:
								position, tokenIndex = position336, tokenIndex336
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
						if !matchDot() {
							goto l334
						}
						goto l333
					l334:
						position, tokenIndex = position333, tokenIndex333
						if buffer[position] != rune('\\') {
							goto l332
						}
						position++
						if buffer[position] != rune(']') {
							goto l332
						}
						position++
					}
				l333:
					goto l331
				l332:
					position, tokenIndex = position332, tokenIndex332
				}
				add(ruledefault, position330)
			}
			return true
		},
		/* 28 cardinality_right <- <cardinality> */
		func() bool {
			position339, tokenIndex339 := position, tokenIndex
			{
				position340 := position
				if !_rules[rulecardinality]() {
					goto l339
				}
				add(rulecardinality_right, position340)
			}
			return true
		l339:
			position, tokenIndex = position339, tokenIndex339
			return false
		},
		/* 29 cardinality_left <- <cardinality> */
		func() bool {
			position341, tokenIndex341 := position, tokenIndex
			{
				position342 := position
				if !_rules[rulecardinality]() {
					goto l341
				}
				add(rulecardinality_left, position342)
			}
			return true
		l341:
			position, tokenIndex = position341, tokenIndex341
			return false
		},
		/* 30 cardinality <- <(('0' / '1' / '*') (. . ('0' / '1' / '*'))?)> */
		func() bool {
			position343, tokenIndex343 := position, tokenIndex
			{
				position344 := position
				{
					position345, tokenIndex345 := position, tokenIndex
					if buffer[position] != rune('0') {
						goto l346
					}
					position++
					goto l345
				l346:
					position, tokenIndex = position345, tokenIndex345
					if buffer[position] != rune('1') {
						goto l347
					}
					position++
					goto l345
				l347:
					position, tokenIndex = position345, tokenIndex345
					if buffer[position] != rune('*') {
						goto l343
					}
					position++
				}
			l345:
				{
					position348, tokenIndex348 := position, tokenIndex
					if !matchDot() {
						goto l348
					}
					if !matchDot() {
						goto l348
					}
					{
						position350, tokenIndex350 := position, tokenIndex
						if buffer[position] != rune('0') {
							goto l351
						}
						position++
						goto l350
					l351:
						position, tokenIndex = position350, tokenIndex350
						if buffer[position] != rune('1') {
							goto l352
						}
						position++
						goto l350
					l352:
						position, tokenIndex = position350, tokenIndex350
						if buffer[position] != rune('*') {
							goto l348
						}
						position++
					}
				l350:
					goto l349
				l348:
					position, tokenIndex = position348, tokenIndex348
				}
			l349:
				add(rulecardinality, position344)
			}
			return true
		l343:
			position, tokenIndex = position343, tokenIndex343
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
