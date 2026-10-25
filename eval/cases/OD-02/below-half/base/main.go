package main
type base0 interface { base0Method() }
type base00 struct{}
func (base00) base0Method() {}
type base1 interface { base1Method() }
type base10 struct{}
func (base10) base1Method() {}
type base2 interface { base2Method() }
type base20 struct{}
func (base20) base2Method() {}
type base21 struct{}
func (base21) base2Method() {}
type base3 interface { base3Method() }
type base30 struct{}
func (base30) base3Method() {}
type base31 struct{}
func (base31) base3Method() {}
type base4 interface { base4Method() }
type base40 struct{}
func (base40) base4Method() {}
type base41 struct{}
func (base41) base4Method() {}
