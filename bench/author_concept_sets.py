#!/usr/bin/env python3
"""Author curated concept-set fixtures (diffusion-contrast-retrieval, task 1.1/1.2).

Questions are drafted from framework documentation knowledge — NEVER from
search output. Each question carries an any-of-N acceptable-answer set.
This script verifies every accept against the repo's symbol table (direct
SQL existence lookup — not the search pipeline), prunes accepts that don't
exist in the pinned index, drops questions left with no accepts, and writes
the frozen fixture. Run once per repo; fixtures are then FROZEN — edits
after mechanisms run cost a registered iteration.

Usage: python3 author_concept_sets.py --repo <path> --name <gin|flask|nest|laravel-framework|...> --split tuning|heldout
"""

from __future__ import annotations

import argparse
import json
import sqlite3
import subprocess
from datetime import date
from pathlib import Path

# Draft question banks. accept entries are "Name" or "Parent.Name".
DRAFTS: dict[str, list[dict]] = {
    "gin": [
        {"q": "serve static files from a directory over http", "accept": ["Static", "StaticFS", "StaticFile", "RouterGroup.Static", "RouterGroup.StaticFS", "RouterGroup.StaticFile"]},
        {"q": "bind the incoming json request body onto a struct", "accept": ["ShouldBindJSON", "BindJSON", "ShouldBind", "ShouldBindWith", "MustBindWith", "Context.ShouldBindJSON", "Context.BindJSON"]},
        {"q": "send a json response to the client", "accept": ["JSON", "IndentedJSON", "PureJSON", "SecureJSON", "AsciiJSON", "Context.JSON"]},
        {"q": "group several routes under a common url prefix", "accept": ["Group", "RouterGroup.Group"]},
        {"q": "register middleware that runs for every request", "accept": ["Use", "Engine.Use", "RouterGroup.Use"]},
        {"q": "stop processing the request from inside middleware", "accept": ["Abort", "AbortWithStatus", "AbortWithStatusJSON", "AbortWithError", "Context.Abort"]},
        {"q": "read a path parameter like the id in /users/:id", "accept": ["Param", "Context.Param", "Params.Get", "Params.ByName"]},
        {"q": "read a query string parameter with a fallback default", "accept": ["Query", "DefaultQuery", "GetQuery", "Context.Query", "Context.DefaultQuery"]},
        {"q": "figure out the real client ip address behind proxies", "accept": ["ClientIP", "RemoteIP", "Context.ClientIP", "Context.RemoteIP"]},
        {"q": "set a cookie on the http response", "accept": ["SetCookie", "Context.SetCookie"]},
        {"q": "recover from a panic in a handler and return a 500", "accept": ["Recovery", "RecoveryWithWriter", "CustomRecovery", "CustomRecoveryWithWriter"]},
        {"q": "log each request with latency and status to the console", "accept": ["Logger", "LoggerWithConfig", "LoggerWithFormatter", "LoggerWithWriter"]},
        {"q": "load html templates so handlers can render them", "accept": ["LoadHTMLGlob", "LoadHTMLFiles", "Engine.LoadHTMLGlob", "Engine.LoadHTMLFiles", "SetHTMLTemplate"]},
        {"q": "receive an uploaded file from a multipart form", "accept": ["FormFile", "MultipartForm", "SaveUploadedFile", "Context.FormFile", "Context.MultipartForm"]},
        {"q": "redirect the client to a different url", "accept": ["Redirect", "Context.Redirect"]},
        {"q": "start the server with tls certificates", "accept": ["RunTLS", "Engine.RunTLS"]},
        {"q": "pick the response format based on the accept header", "accept": ["Negotiate", "NegotiateFormat", "Context.Negotiate", "Context.NegotiateFormat"]},
        {"q": "register one handler for all http methods on a path", "accept": ["Any", "RouterGroup.Any", "Handle", "RouterGroup.Handle"]},
        {"q": "stream server sent events to the client", "accept": ["Stream", "SSEvent", "Context.Stream", "Context.SSEvent"]},
        {"q": "store a value on the request context and read it back later", "accept": ["Set", "Get", "MustGet", "Context.Set", "Context.Get", "Context.MustGet"]},
        {"q": "serve a single file when a route is hit", "accept": ["File", "Context.File", "StaticFile", "FileAttachment"]},
        {"q": "customize the handler for unmatched routes", "accept": ["NoRoute", "NoMethod", "Engine.NoRoute", "Engine.NoMethod"]},
        {"q": "run the server on a unix domain socket or existing listener", "accept": ["RunUnix", "RunListener", "RunFd", "Engine.RunUnix", "Engine.RunListener"]},
        {"q": "read the raw request body bytes", "accept": ["GetRawData", "Context.GetRawData"]},
        {"q": "protect routes with http basic authentication", "accept": ["BasicAuth", "BasicAuthForRealm"]},
        {"q": "configure which proxies to trust for forwarded headers", "accept": ["SetTrustedProxies", "Engine.SetTrustedProxies", "isTrustedProxy"]},
    ],
    "flask": [
        {"q": "register a url route for a view function", "accept": ["route", "add_url_rule", "Flask.add_url_rule", "Scaffold.route"]},
        {"q": "how a request gets dispatched to its view function", "accept": ["dispatch_request", "full_dispatch_request", "wsgi_app", "Flask.dispatch_request", "Flask.full_dispatch_request", "Flask.wsgi_app"]},
        {"q": "redirect the user to another page", "accept": ["redirect"]},
        {"q": "build the url for an endpoint by name", "accept": ["url_for", "Flask.url_for"]},
        {"q": "render an html template with variables", "accept": ["render_template", "render_template_string"]},
        {"q": "keep user data across requests in a session", "accept": ["SecureCookieSession", "SecureCookieSessionInterface", "open_session", "save_session", "SessionMixin"]},
        {"q": "run a function before every request", "accept": ["before_request", "Scaffold.before_request", "Flask.preprocess_request", "preprocess_request"]},
        {"q": "register a handler for a specific http error like 404", "accept": ["errorhandler", "register_error_handler", "handle_http_exception", "Scaffold.errorhandler", "Flask.register_error_handler"]},
        {"q": "abort the request with an http error status", "accept": ["abort", "Flask.aborter", "make_aborter"]},
        {"q": "flash a one time message shown on the next page", "accept": ["flash", "get_flashed_messages"]},
        {"q": "split an application into reusable route groups", "accept": ["Blueprint", "register_blueprint", "Flask.register_blueprint", "BlueprintSetupState"]},
        {"q": "safely send a file from a directory as the response", "accept": ["send_file", "send_from_directory"]},
        {"q": "parse json from the incoming request body", "accept": ["get_json", "Request.get_json"]},
        {"q": "return a json response from a view", "accept": ["jsonify", "JSONProvider.response", "DefaultJSONProvider.response"]},
        {"q": "load app configuration from an object or file", "accept": ["from_object", "from_pyfile", "from_envvar", "from_prefixed_env", "Config.from_object", "Config.from_pyfile"]},
        {"q": "start the built in development server", "accept": ["run", "Flask.run"]},
        {"q": "push an application context to use the app outside a request", "accept": ["app_context", "AppContext", "Flask.app_context", "AppContext.push"]},
        {"q": "the object that holds request state during handling", "accept": ["RequestContext", "request_context", "Flask.request_context", "RequestContext.push"]},
        {"q": "keep the request context alive while streaming a response", "accept": ["stream_with_context"]},
        {"q": "modify the response after the view runs", "accept": ["after_request", "process_response", "Scaffold.after_request", "Flask.process_response"]},
        {"q": "clean up resources when the request ends", "accept": ["teardown_request", "teardown_appcontext", "do_teardown_request", "do_teardown_appcontext"]},
        {"q": "add custom command line commands to the app", "accept": ["AppGroup", "FlaskGroup", "with_appcontext", "add_command"]},
        {"q": "make test requests against the app in unit tests", "accept": ["test_client", "FlaskClient", "Flask.test_client", "EnvironBuilder"]},
        {"q": "register a custom filter usable inside templates", "accept": ["template_filter", "add_template_filter", "Scaffold.template_filter", "Flask.add_template_filter"]},
        {"q": "automatic response for options requests", "accept": ["make_default_options_response", "Flask.make_default_options_response"]},
        {"q": "turn a view's return value into a response object", "accept": ["make_response", "Flask.make_response"]},
    ],
    "nest": [
        {"q": "mark a class as a provider that can be injected", "accept": ["Injectable"]},
        {"q": "mark a class as an http request controller", "accept": ["Controller"]},
        {"q": "declare a route method that handles get requests", "accept": ["Get", "createMappingDecorator", "RequestMapping"]},
        {"q": "read a path parameter inside a handler", "accept": ["Param"]},
        {"q": "read the parsed request body in a handler", "accept": ["Body"]},
        {"q": "group providers and controllers into a unit", "accept": ["Module"]},
        {"q": "throw an error meaning the resource was not found", "accept": ["NotFoundException"]},
        {"q": "the base class all http errors derive from", "accept": ["HttpException"]},
        {"q": "decide whether a request is allowed to proceed", "accept": ["CanActivate", "UseGuards", "GuardsConsumer", "GuardsContextCreator"]},
        {"q": "wrap handler execution to run logic before and after", "accept": ["NestInterceptor", "UseInterceptors", "InterceptorsConsumer", "InterceptorsContextCreator"]},
        {"q": "transform and validate handler arguments", "accept": ["PipeTransform", "UsePipes", "ValidationPipe", "PipesConsumer"]},
        {"q": "catch thrown exceptions and shape the error response", "accept": ["ExceptionFilter", "Catch", "UseFilters", "BaseExceptionFilter", "ExceptionsHandler"]},
        {"q": "bootstrap and start the application", "accept": ["NestFactory", "NestFactoryStatic", "NestApplication"]},
        {"q": "resolve a provider's dependencies and construct it", "accept": ["Injector", "NestContainer", "InstanceLoader", "InstanceWrapper"]},
        {"q": "build your own decorator that extracts data from the request", "accept": ["createParamDecorator"]},
        {"q": "handle a microservice message by pattern", "accept": ["MessagePattern", "EventPattern"]},
        {"q": "receive websocket messages in a gateway class", "accept": ["WebSocketGateway", "SubscribeMessage"]},
        {"q": "apply middleware to selected routes of a module", "accept": ["MiddlewareConsumer", "NestMiddleware", "MiddlewareModule"]},
        {"q": "run code when a module finishes initializing", "accept": ["OnModuleInit", "OnApplicationBootstrap", "callModuleInitHook"]},
        {"q": "prefix every route in the app with a base path", "accept": ["setGlobalPrefix", "GlobalPrefixOptions"]},
        {"q": "read custom metadata attached by decorators", "accept": ["Reflector", "SetMetadata"]},
        {"q": "return a specific http status code from a handler", "accept": ["HttpCode"]},
        {"q": "stream a file back as the response", "accept": ["StreamableFile"]},
        {"q": "expose multiple versions of the same route", "accept": ["enableVersioning", "VersioningOptions", "RouterModule"]},
        {"q": "load a module lazily at runtime", "accept": ["LazyModuleLoader"]},
        {"q": "scan module imports to build the dependency graph", "accept": ["DependenciesScanner", "scanForModules"]},
    ],
    "laravel-framework": [
        {"q": "roll back the last batch of database migrations", "accept": ["rollback", "rollbackMigrations", "RollbackCommand", "Migrator.rollback"]},
        {"q": "run the pending database migrations", "accept": ["Migrator.run", "runPending", "MigrateCommand", "runUp"]},
        {"q": "define the columns of a new database table", "accept": ["Blueprint", "Builder.create", "Blueprint.build"]},
        {"q": "add a where condition to a database query", "accept": ["Builder.where", "addBasicWhereClause", "Builder.orWhere"]},
        {"q": "persist a model instance to the database", "accept": ["Model.save", "performInsert", "performUpdate", "Model.saveOrFail"]},
        {"q": "declare that a model has many related child models", "accept": ["hasMany", "HasMany", "HasOneOrMany", "newHasMany"]},
        {"q": "declare the inverse relation where a model belongs to a parent", "accept": ["belongsTo", "BelongsTo", "newBelongsTo"]},
        {"q": "push a job onto the queue for background processing", "accept": ["dispatch", "PendingDispatch", "Queue.push", "Dispatcher.dispatch", "dispatchToQueue"]},
        {"q": "retry jobs that previously failed", "accept": ["RetryCommand", "retryJob", "FailedJobProviderInterface"]},
        {"q": "fire an event and notify its listeners", "accept": ["Dispatcher.dispatch", "Dispatcher.listen", "invokeListeners", "Dispatcher.until"]},
        {"q": "validate incoming request data against rules", "accept": ["Validator", "validate", "ValidatesRequests", "FormRequest", "Validator.passes", "Validator.fails"]},
        {"q": "hash a user password for storage", "accept": ["BcryptHasher", "HashManager", "BcryptHasher.make", "Argon2IdHasher", "AbstractHasher"]},
        {"q": "encrypt a value and decrypt it later", "accept": ["Encrypter", "Encrypter.encrypt", "Encrypter.decrypt", "encryptString", "decryptString"]},
        {"q": "register a route with the router", "accept": ["Router.get", "Router.post", "Router.addRoute", "RouteRegistrar", "Router.match"]},
        {"q": "resolve a model instance from a route parameter automatically", "accept": ["ImplicitRouteBinding", "resolveForRouteBinding", "substituteImplicitBindings", "resolveRouteBinding"]},
        {"q": "pass the request through the middleware chain", "accept": ["Pipeline", "Pipeline.then", "sendRequestThroughRouter", "Pipeline.through"]},
        {"q": "send an email message", "accept": ["Mailer.send", "Mailable", "Mailer", "Mailable.send", "SendQueuedMailable"]},
        {"q": "cache a computed value and reuse it until it expires", "accept": ["remember", "Repository.remember", "rememberForever", "Repository.get"]},
        {"q": "schedule recurring console tasks like cron", "accept": ["Schedule", "Schedule.command", "ScheduleRunCommand", "Schedule.call"]},
        {"q": "define a custom artisan console command", "accept": ["Command", "Command.handle", "GeneratorCommand", "Command.execute"]},
        {"q": "bind an implementation into the service container and resolve it", "accept": ["Container.bind", "Container.make", "Container.resolve", "Container.singleton"]},
        {"q": "register services when the framework boots", "accept": ["ServiceProvider", "ServiceProvider.register", "ServiceProvider.boot", "bootProviders", "registerConfiguredProviders"]},
        {"q": "compile blade templates into php", "accept": ["BladeCompiler", "BladeCompiler.compile", "compileString", "compileStatements"]},
        {"q": "paginate query results into pages", "accept": ["paginate", "Paginator", "LengthAwarePaginator", "CursorPaginator", "Builder.paginate"]},
        {"q": "run several database statements in one transaction", "accept": ["transaction", "beginTransaction", "Connection.transaction", "ManagesTransactions"]},
        {"q": "soft delete models instead of removing rows", "accept": ["SoftDeletes", "SoftDeletingScope", "runSoftDelete", "restore"]},
    ],
}


def verify(repo: Path, accepts: list[str]) -> list[str]:
    """Keep accepts that exist in the symbol table (direct lookup only)."""
    con = sqlite3.connect(str(repo / ".codeindex" / "graph.db"))
    kept = []
    for a in accepts:
        if "." in a:
            parent, name = a.rsplit(".", 1)
            n = con.execute(
                "SELECT COUNT(*) FROM symbols WHERE tier=0 AND name=? AND parent=?",
                (name, parent)).fetchone()[0]
        else:
            n = con.execute(
                "SELECT COUNT(*) FROM symbols WHERE tier=0 AND name=?", (a,)).fetchone()[0]
        if n > 0:
            kept.append(a)
    con.close()
    return kept


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--repo", required=True)
    ap.add_argument("--name", required=True)
    ap.add_argument("--split", required=True, choices=["tuning", "heldout"])
    ap.add_argument("--drafts", help="external drafts JSON (for held-out repos)")
    args = ap.parse_args()

    repo = Path(args.repo)
    drafts = DRAFTS.get(args.name)
    if args.drafts:
        drafts = json.loads(Path(args.drafts).read_text())
    if not drafts:
        raise SystemExit(f"no drafts for {args.name}")

    commit = subprocess.run(["git", "-C", str(repo), "rev-parse", "HEAD"],
                            capture_output=True, text=True).stdout.strip()

    kept, dropped = [], []
    for d in drafts:
        acc = verify(repo, d["accept"])
        if acc:
            kept.append({"q": d["q"], "accept": acc})
        else:
            dropped.append(d["q"])

    out = {
        "repo": args.name,
        "commit": commit,
        "split": args.split,
        "provenance": (
            "Drafted from framework documentation knowledge by claude-fable-5 "
            f"on {date.today().isoformat()}; answers verified against the pinned "
            "index via symbol-table existence lookup only (no search runs); "
            "frozen before diffusion/contrast mechanisms were measured."
        ),
        "questions": kept,
    }
    dst = Path(__file__).parent / "concept_sets" / f"{args.name}.json"
    dst.parent.mkdir(exist_ok=True)
    dst.write_text(json.dumps(out, indent=1))
    print(f"{args.name}: kept {len(kept)} questions, dropped {len(dropped)}: {dropped}")


if __name__ == "__main__":
    main()
