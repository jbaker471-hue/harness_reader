package main

import (
 "reflect"
 "strconv"
 "testing"
)

func planFixture() PlanConnector {
 c := PlanConnector{Tag:"P1",Part:"D38999/26FG35SA",AllVerified:true,AllSource:"synthetic verified inventory",Wired:[]PlanWire{{Cavity:"1",AWG:"24"}}}
 for i:=1;i<=79;i++ { c.All=append(c.All,strconv.Itoa(i)) }
 return c
}
func makeTestPlan(t *testing.T,c PlanConnector,policy string,bom []AccBOM) BuildPlan {
 t.Helper()
 p,e:=MakeBuildPlan(PlanRequest{BuildQty:3,Policy:policy,Connectors:[]PlanConnector{c},BOM:bom})
 if e!=nil {t.Fatal(e)}
 return p
}
func TestBuildPlanPolicyAndMultiplier(t *testing.T) {
 for _,tc:=range []struct{policy string; contacts int}{{"contact+plug",79},{"plug-only",1}} {
  p:=makeTestPlan(t,planFixture(),tc.policy,nil)
  for _,v:=range p.Totals {
   want:=78
   if v.Part=="M39029/56-348" { want=tc.contacts }
   if v.Need!=want || v.NeedBuild!=want*3 {t.Fatalf("%s: %+v",tc.policy,v)}
  }
  cr:=p.Connectors[0].Crimp
  if len(cr)!=1 || cr[0].Status!="OK" || cr[0].Frame=="UNRESOLVED" || cr[0].Locator=="UNRESOLVED" { t.Fatalf("tooling: %+v",cr) }
 }
}
func TestBuildPlanAWGAndOverride(t *testing.T) {
 for _,tc:=range []struct{awg,status string}{{"20","OUT OF RANGE"},{"","AWG UNKNOWN"},{"24","OK"}} {
  c:=planFixture();c.Contact="M39029/56-348";c.Wired[0].AWG=tc.awg
  p:=makeTestPlan(t,c,"plug-only",nil)
  if p.Connectors[0].ContactSource!="user entry" || p.Connectors[0].Crimp[0].Status!=tc.status {t.Fatalf("%+v",p.Connectors[0])}
 }
}
func TestBuildPlanUncertainInventoryNeverDefinitive(t *testing.T) {
 for _,kind:=range []string{"assumed","unknown","mismatch","duplicate","outside","wired-unused"} {
  c:=planFixture()
  switch kind {
  case "assumed":c.AllVerified=false
  case "unknown":c.All=nil
  case "mismatch":c.All=c.All[:2]
  case "duplicate":c.All=append(c.All,"1")
  case "outside":c.Wired[0].Cavity="999"
  case "wired-unused":c.Unused=[]string{"1"}
  }
  p:=makeTestPlan(t,c,"contact+plug",nil)
  if p.Connectors[0].UnusedComplete {t.Fatalf("%s falsely complete",kind)}
  for _,v:=range p.Totals {if v.Need!=-1 || v.NeedBuild!=-1 {t.Fatalf("%s: %+v",kind,v)}}
 }
}
func TestBuildPlanWrongContactAndPlug(t *testing.T) {
 p:=makeTestPlan(t,planFixture(),"contact+plug",[]AccBOM{{Part:"M39029/58-360",Qty:"79"},{Part:"MS27488-220",Qty:"78"}})
 for _,v:=range p.Totals {if v.Status!="WRONG PART" {t.Fatalf("%+v",v)}}
}
func TestUnusedCavityLetterCase(t *testing.T) {
 got:=parseCavityList("A, a and b, 7 thru 9 TO BE FILLED")
 want:=[]string{"A","a","b","7","8","9"}
 if !reflect.DeepEqual(got,want) {t.Fatalf("got %v want %v",got,want)}
}
