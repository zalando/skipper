package policy

default allow = false

allow if {
  is_admin(data.roles, input.parsed_path[1])
}

is_admin(roles, user) = true if {
  roles[user] == "admin"
}
