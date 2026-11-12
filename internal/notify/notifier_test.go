package notify

import (
	"testing"

	"gitlens/ent"
	"gitlens/ent/event"
)

func TestShouldNotify(t *testing.T) {
	allOn := &ent.User{
		NotificationsEnabled:  true,
		NotifyWeb:             true,
		NotifyAndroid:         true,
		NotifyRelease:         true,
		NotifyPrMerge:         true,
		NotifyWorkflowFailure: true,
	}
	allOnCfg := func() *ent.AdminConfig {
		return &ent.AdminConfig{
			NotificationsEnabled:  true,
			NotifyRelease:         true,
			NotifyPrMerge:         true,
			NotifyWorkflowFailure: true,
		}
	}

	cases := []struct {
		name   string
		cfg    *ent.AdminConfig
		user   *ent.User
		typeOf event.Type
		want   bool
	}{
		{
			name:   "all on release",
			cfg:    allOnCfg(),
			user:   allOn,
			typeOf: event.TypeRelease,
			want:   true,
		},
		{
			name:   "nil config defaults enabled",
			cfg:    nil,
			user:   allOn,
			typeOf: event.TypePrMerge,
			want:   true,
		},
		{
			name: "global admin switch off",
			cfg: func() *ent.AdminConfig {
				c := allOnCfg()
				c.NotificationsEnabled = false
				return c
			}(),
			user:   allOn,
			typeOf: event.TypeRelease,
			want:   false,
		},
		{
			name: "admin type default off",
			cfg: func() *ent.AdminConfig {
				c := allOnCfg()
				c.NotifyWorkflowFailure = false
				return c
			}(),
			user:   allOn,
			typeOf: event.TypeWorkflowFailure,
			want:   false,
		},
		{
			name: "user type preference off",
			cfg:  allOnCfg(),
			user: func() *ent.User {
				u := *allOn
				u.NotifyPrMerge = false
				return &u
			}(),
			typeOf: event.TypePrMerge,
			want:   false,
		},
		{
			name: "user master switch off",
			cfg:  allOnCfg(),
			user: func() *ent.User {
				u := *allOn
				u.NotificationsEnabled = false
				return &u
			}(),
			typeOf: event.TypeRelease,
			want:   false,
		},
		{
			name:   "unknown event type",
			cfg:    allOnCfg(),
			user:   allOn,
			typeOf: event.Type("other"),
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldNotify(tc.cfg, tc.user, tc.typeOf); got != tc.want {
				t.Fatalf("shouldNotify = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldNotifyNilUser(t *testing.T) {
	if shouldNotify(nil, nil, event.TypeRelease) {
		t.Fatal("nil user must never notify")
	}
}
