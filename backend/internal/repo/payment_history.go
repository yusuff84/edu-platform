package repo

func (s *Store) HasCourseBillingHistory(courseID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.payments {
		if p.CourseID == courseID {
			return true, nil
		}
	}
	for _, e := range s.enrollments {
		if e.CourseID == courseID {
			return true, nil
		}
	}
	return false, nil
}

func (s *PostgresStore) HasCourseBillingHistory(courseID string) (bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM payments WHERE course_id=$1::uuid)
	 OR EXISTS(SELECT 1 FROM enrollments WHERE course_id=$1::uuid)`, courseID).Scan(&exists)
	return exists, err
}
