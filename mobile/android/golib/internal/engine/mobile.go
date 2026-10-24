package engine

// CloseGone closes both TCP and UDP sockets on removed physical networks.
func (s *Scheduler) CloseGone() {
	s.mu.Lock()
	var gone []*Slot
	for _, sl := range s.slots {
		if !sl.present {
			gone = append(gone, sl)
		}
	}
	s.mu.Unlock()
	for _, sl := range gone {
		sl.closeFlows()
	}
}
