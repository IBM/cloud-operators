
function ts::run() {
    u::begin_testcase "should deploy sample translator service"

    kubectl apply -f service-translator.yaml 
    object::wait_service_online test-translator-1 100

    u::end_testcase 
}

function ts::cleanup() {
    kubectl delete -f service-translator.yaml 
}