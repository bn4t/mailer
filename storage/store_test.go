package storage

import (
	"testing"

	"mailer/models"
)

func TestDeleteAllNeverReusesIDs(t *testing.T) {
	store := NewStore()

	first := store.Save(&models.Email{Subject: "first"})
	second := store.Save(&models.Email{Subject: "second"})
	if first != 1 || second != 2 {
		t.Fatalf("initial IDs = %d, %d, want 1, 2", first, second)
	}

	store.DeleteAll()
	if store.Count() != 0 {
		t.Fatalf("Count after DeleteAll = %d, want 0", store.Count())
	}

	third := store.Save(&models.Email{Subject: "third"})
	if third != 3 {
		t.Errorf("ID after DeleteAll = %d, want 3 (IDs must not be reused for IMAP UIDs)", third)
	}
	if store.NextID() != 4 {
		t.Errorf("NextID = %d, want 4", store.NextID())
	}
}

func TestGetAllIsSortedByID(t *testing.T) {
	store := NewStore()
	store.Save(&models.Email{Subject: "first"})
	store.Save(&models.Email{Subject: "second"})
	store.Save(&models.Email{Subject: "third"})
	store.Delete(2)

	emails := store.GetAll()
	if len(emails) != 2 {
		t.Fatalf("GetAll returned %d emails, want 2", len(emails))
	}
	if emails[0].ID != 1 || emails[1].ID != 3 {
		t.Errorf("GetAll IDs = %d, %d, want 1, 3", emails[0].ID, emails[1].ID)
	}
}
