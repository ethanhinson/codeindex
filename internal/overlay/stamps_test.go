package overlay

import "testing"

func TestPutStampAndStampRoundTrip(t *testing.T) {
	s := newStore(t)
	if err := s.PutStamp("app", "root1"); err != nil {
		t.Fatalf("PutStamp: %v", err)
	}
	got, ok, err := s.Stamp("app")
	if err != nil {
		t.Fatalf("Stamp: %v", err)
	}
	if !ok {
		t.Fatalf("Stamp(app) ok = false, want true")
	}
	if got.MemberID != "app" || got.MerkleRoot != "root1" {
		t.Fatalf("Stamp(app) = %+v, want MemberID=app MerkleRoot=root1", got)
	}
	if got.ResolvedAt == 0 {
		t.Fatalf("Stamp(app).ResolvedAt = 0, want a unix timestamp")
	}
}

func TestStampAbsentMember(t *testing.T) {
	s := newStore(t)
	got, ok, err := s.Stamp("missing")
	if err != nil {
		t.Fatalf("Stamp: %v", err)
	}
	if ok {
		t.Fatalf("Stamp(missing) ok = true, want false")
	}
	if got != (Stamp{}) {
		t.Fatalf("Stamp(missing) = %+v, want zero value", got)
	}
}

func TestPutStampOverwritesNotDuplicates(t *testing.T) {
	s := newStore(t)
	if err := s.PutStamp("app", "root1"); err != nil {
		t.Fatalf("PutStamp: %v", err)
	}
	if err := s.PutStamp("app", "root2"); err != nil {
		t.Fatalf("PutStamp: %v", err)
	}
	if n := countQ(t, s, `SELECT COUNT(*) FROM member_stamps`); n != 1 {
		t.Fatalf("member_stamps rows = %d, want 1", n)
	}
	got, ok, err := s.Stamp("app")
	if err != nil {
		t.Fatalf("Stamp: %v", err)
	}
	if !ok || got.MerkleRoot != "root2" {
		t.Fatalf("Stamp(app) = %+v ok=%v, want MerkleRoot=root2 ok=true", got, ok)
	}
}

func TestStampsOrderedByMemberID(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"web", "app", "lib"} {
		if err := s.PutStamp(id, "root-"+id); err != nil {
			t.Fatalf("PutStamp(%s): %v", id, err)
		}
	}
	got, err := s.Stamps()
	if err != nil {
		t.Fatalf("Stamps: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Stamps() len = %d, want 3", len(got))
	}
	want := []string{"app", "lib", "web"}
	for i, id := range want {
		if got[i].MemberID != id {
			t.Fatalf("Stamps()[%d].MemberID = %q, want %q", i, got[i].MemberID, id)
		}
	}
}
