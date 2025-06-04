package policy

default allow = false

allow if {
    user := input.parsed_path[1]
    data.roles[user] == "admin"
}