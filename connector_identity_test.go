package main
import "testing"
func TestPrefixedConnectorIdentity(t *testing.T) {
 for _, tag := range []string{"RACK-J7", "BENCH-P9", "ZONE_A-J12", "P1"} {
  if got:=reRefDesignator.FindString(tag);got!=tag {t.Errorf("tag truncated: %q -> %q",tag,got)}
  if !reDestTag.MatchString(tag) {t.Errorf("endpoint rejected: %q",tag)}
 }
 for _, tag:=range []string{"UNASSIGNED","PART-123","UNRESOLVED ENDPOINT"} {
  if reDestTag.MatchString(tag) {t.Errorf("invalid endpoint accepted: %q",tag)}
 }
}
