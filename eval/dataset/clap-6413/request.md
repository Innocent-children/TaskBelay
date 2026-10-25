derive 属性等号右侧支持以字符串字面量开头的完整表达式，例如 default_value_t = "output.obj".to_string()。省略参数时得到计算后的默认值，显式参数仍覆盖默认值，帮助显示正确默认值。原本只接受字符串字面量的属性（如 rename_all）仍拒绝非字面量表达式，并给出清楚的属性错误；原有字面量写法保持兼容。

对这类非字面量错误，沿用 attribute `<属性名>` can only accept string literals 的诊断文本，错误位置覆盖等号右侧的完整表达式。
