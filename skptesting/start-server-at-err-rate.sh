fail=$1

if [ -z $fail ]
then
	fail = "0.1"
fi

skipper -address :9090 -inline-routes "
f: Traffic(\"$fail\") -> status(500) -> <shunt>;
r: * -> status(200) -> <shunt>;
"