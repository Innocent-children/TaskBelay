package p
type State struct{enabled bool}
func (s State) Enabled() bool{return s.enabled}
func NewState() *State {return &State{enabled:true}}
