增加 Func、FuncP、BoolFunc、BoolFuncP 回调参数，在 FlagSet 和包级入口均可定义。每次遇到参数时按出现顺序调用回调并传入解析值；带 P 的入口支持短参数。普通 Func 需要值，BoolFunc 在没有显式值时传入 true；回调返回错误时按现有参数错误处理返回，并保持现有参数解析行为。
