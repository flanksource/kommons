package ktemplate

import (
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

type MarkdownTable struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

func (f *Functions) ParseMarkdownTables(input string) []MarkdownTable {
	inputByte := []byte(input)
	parser := parser.New()
	output := parser.Parse(inputByte)
	records := extractTextFromNode(output)

	return records
}

func extractMultipleRowCell(node ast.Node) string {
	for _, c := range node.GetChildren() {
		switch cc := c.(type) {
		case *ast.Text:
			content := string(cc.AsLeaf().Literal)
			if content != "" {
				return content
			}
		case *ast.Link:
			return string(cc.Destination)
		}
	}
	return ""
}

func extractTableRow(node ast.Node) [][]string {
	results := [][]string{}

	switch nn := node.(type) {
	case *ast.TableRow:
		var s []string
		for _, c := range nn.GetChildren() {
			if len(c.GetChildren()) == 0 {
				s = append(s, "")
				continue
			}
			cell := extractMultipleRowCell(c)
			s = append(s, cell)
		}
		results = append(results, s)
	default:
		for _, c := range nn.GetChildren() {
			s := extractTableRow(c)
			if len(s) > 0 {
				results = append(results, s...)
			}
		}
	}

	return results
}

func extractTableBody(node *ast.TableBody) [][]string {
	results := [][]string{}
	for _, r := range node.GetChildren() {
		res := extractTableRow(r)
		results = append(results, res...)
	}

	return results
}

func extractTableHeader(node ast.Node) []string {
	colls := []string{}

	for _, c := range node.GetChildren() {
		switch cc := c.(type) {
		case *ast.Text:
			text := string(cc.AsLeaf().Literal)
			if text != "" {
				colls = append(colls, text)
			}
		default:
			for _, ccc := range cc.GetChildren() {
				res := extractTableHeader(ccc)
				colls = append(colls, res...)
			}
		}
	}
	return colls
}

func extractTable(node ast.Node) MarkdownTable {
	table := MarkdownTable{}

	for _, n := range node.GetChildren() {
		switch nn := n.(type) {
		case *ast.TableHeader:
			col := extractTableHeader(nn)
			table.Columns = col
		case *ast.TableBody:
			table.Rows = extractTableBody(nn)
		}
	}

	return table
}

func extractTextFromNode(node ast.Node) []MarkdownTable {
	switch node := node.(type) {
	case *ast.Document:
		tables := []MarkdownTable{}

		for _, n := range node.GetChildren() {
			if tt := extractTextFromNode(n); tt != nil {
				tables = append(tables, tt...)
			}
		}
		return tables
	case *ast.Table:
		table := extractTable(node)
		return []MarkdownTable{table}
	default:
		return nil
	}
}
