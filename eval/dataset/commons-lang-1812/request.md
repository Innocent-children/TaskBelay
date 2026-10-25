TypeUtils.containsTypeVariables 与 unrollVariables 需要正确处理 ParameterizedType 的 owner。Outer<T>.Inner 应识别外层变量，并按调用方提供的映射展开外层和内层变量，不能用自映射覆盖已有 T 到 String 的决定。普通 raw Class owner 不应仅因声明泛型而被误判；相同变量在不同位置出现时应分别展开。

自定义类型的 owner、类型参数、通配符边界与变量映射可能形成自引用、相互引用或混合循环，检查和展开必须终止，也不能遗漏循环旁的独立变量。owner 链本身成环的参数化类型保留原对象及其参数；其他分支遇到循环时保留循环处的原引用，继续处理旁边不成环的分支。参数化类型的 owner 和参数均未变化，或通配符的上下界均未变化时，返回原对象，不另造等价对象。已有简单类型行为保持兼容。
