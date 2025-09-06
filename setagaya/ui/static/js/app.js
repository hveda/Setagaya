var routes = [
    {
        path: "/",
        component: Projects
    },
    {
        path: "/collections/:id",
        component: Collection
    },
    {
        path: "/plans/:id",
        component: Plan
    },

];

var router = new VueRouter(
    {
        routes: routes
    }
)
var setagaya = new Vue({
    router: router,
})

setagaya.$mount(".setagaya")
