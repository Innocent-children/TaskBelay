ClassUtils.getShortClassName(Class) 处理匿名类或局部类中的成员类时，保留完整的外部类上下文和编译器编号，不能得到 .Inner 或丢失外层的 Local.Inner。普通成员类、叶节点匿名或局部类以及名称包含 $ 的既有行为保持不变。
