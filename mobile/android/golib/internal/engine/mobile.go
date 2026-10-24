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

// CloseAdapter closes sockets pinned to an Android network handle which was replaced.
// Stable adapter counters and allowances are deliberately retained across that replacement.
func (s *Scheduler) CloseAdapter(id string) {
	s.mu.Lock()
	var found *Slot
	for _, sl := range s.slots {
		if sl.key == id {
			found = sl
			break
		}
	}
	s.mu.Unlock()
	if found != nil {
		found.closeFlows()
	}
}
