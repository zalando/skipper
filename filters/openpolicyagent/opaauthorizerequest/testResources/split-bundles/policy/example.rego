package policy

default allow = false

allow if {
    data.roles[input.user] == "admin"
}