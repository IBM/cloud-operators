
function tb::run() {
    u::begin_testcase "should deploy sample translator binding"

    kubectl apply -f binding-translator.yaml 
    object::wait_binding_online binding-translator 100
    object::check_resource_created secret binding-translator

    u::end_testcase 
}

function tb::cleanup() {
    kubectl delete -f binding-translator.yaml 
}