package lang

/**
 * 这些查询只补充通用具名定义，不为未列出的语言增加入口、框架或测试规则。
 */
func GenericTags(name string) string {
	switch name {
	case "c", "cpp":
		query := `
(declaration declarator: (_) @declarator) @definition.variable
(function_definition declarator: (_) @declarator) @definition.function
(type_definition declarator: (_) @declarator) @definition.type
(enum_specifier name: (type_identifier) @name) @definition.type
`
		if name == "cpp" {
			query += `(alias_declaration name: (type_identifier) @name) @definition.type`
		}
		return query
	case "c_sharp":
		return `
(variable_declarator name: (identifier) @name) @definition.variable
(record_declaration name: (identifier) @name) @definition.class
`
	case "ruby":
		return `
(assignment left: (constant) @name) @definition.constant
(assignment left: (identifier) @name) @definition.variable
(assignment left: (left_assignment_list) @binding) @definition.variable
`
	case "php":
		return `
(assignment_expression left: (_) @binding) @definition.variable
(const_element (name) @name) @definition.constant
`
	}
	return ""
}
