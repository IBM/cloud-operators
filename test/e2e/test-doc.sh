
function td::run() {
    u::begin_testcase "should deploy documentation examples"

    kubectl apply -f greetings.yaml 
    object::wait_function_online greetings 10

    result=$(ibmcloud wsk action invoke greetings -br)
    u::assert_equal '{    "message": "Bonjour"}' "$result"

    u::end_testcase 
}

function td::cleanup() {
    kubectl delete -f greetings.yaml 
}