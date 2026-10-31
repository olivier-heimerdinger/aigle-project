//go:build postgres

// Active le pilote PostgreSQL (lib/pq) lors de la compilation de l'image Docker/K8s (-tags postgres).
package database

import (
	_ "github.com/lib/pq"
)

