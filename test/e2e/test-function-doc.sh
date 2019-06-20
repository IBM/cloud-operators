
function td::run() {
    u::begin_testcase "should deploy function documentation examples"

    kubectl apply -f function-greetings.yaml 
    object::wait_function_online greetings 10

    result=$(ibmcloud wsk action invoke greetings -br)
    u::assert_equal '{    "message": "Bonjour"}' "$result"

    u::end_testcase 
}

function td::cleanup() {
    kubectl delete -f function-greetings.yaml 
}