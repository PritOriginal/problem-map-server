# github.com/PritOriginal/problem-map-server

REST generated docs.

## Routes

<details>
<summary>`/auth/signin`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/auth**
	- **/signin**
		- _POST_
			- [github.com/PritOriginal/problem-map-server/internal/handler/auth.Register.func1.(*handler).SignIn.2]()

</details>
<details>
<summary>`/auth/signup`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/auth**
	- **/signup**
		- _POST_
			- [github.com/PritOriginal/problem-map-server/internal/handler/auth.Register.func1.(*handler).SignUp.1]()

</details>
<details>
<summary>`/auth/tokens/refresh`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/auth**
	- **/tokens/refresh**
		- _POST_
			- [github.com/PritOriginal/problem-map-server/internal/handler/auth.Register.func1.(*handler).RefreshTokens.3]()

</details>
<details>
<summary>`/checks`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/checks**
	- **/**
		- _POST_
			- [github.com/PritOriginal/problem-map-server/internal/handler/checks.Register.func1.1.Verifier.Verify.3]()
			- [github.com/PritOriginal/problem-map-server/internal/handler/checks.Register.func1.1.Authenticator.1]()
			- [github.com/PritOriginal/problem-map-server/internal/handler/checks.Register.func1.1.(*handler).AddCheck.2]()

</details>
<details>
<summary>`/checks/mark/{markId}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/checks**
	- **/mark/{markId}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/checks.Register.func1.(*handler).GetChecksByMarkId.3]()

</details>
<details>
<summary>`/checks/user/{userId}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/checks**
	- **/user/{userId}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/checks.Register.func1.(*handler).GetChecksByUserId.4]()

</details>
<details>
<summary>`/checks/{id}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/checks**
	- **/{id}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/checks.Register.func1.(*handler).GetCheckById.2]()

</details>
<details>
<summary>`/map/cities`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/map**
	- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.New.1]()
	- **/cities**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.(*handler).GetCities.3]()

</details>
<details>
<summary>`/map/districts`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/map**
	- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.New.1]()
	- **/districts**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.(*handler).GetDistricts.4]()

</details>
<details>
<summary>`/map/regions`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/map**
	- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.New.1]()
	- **/regions**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.(*handler).GetRegions.2]()

</details>
<details>
<summary>`/marks`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/marks**
	- **/**
		- _POST_
			- [github.com/PritOriginal/problem-map-server/internal/handler/marks.Register.func1.1.Verifier.Verify.3]()
			- [github.com/PritOriginal/problem-map-server/internal/handler/marks.Register.func1.1.Authenticator.1]()
			- [github.com/PritOriginal/problem-map-server/internal/handler/marks.Register.func1.1.(*handler).AddMark.2]()
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/marks.Register.func1.(*handler).GetMarks.3]()

</details>
<details>
<summary>`/marks/statuses`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/marks**
	- **/statuses**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/marks.Register.func1.2.New.1]()
			- [github.com/PritOriginal/problem-map-server/internal/handler/marks.Register.func1.2.(*handler).GetMarkStatuses.3]()

</details>
<details>
<summary>`/marks/types`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/marks**
	- **/types**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/marks.Register.func1.2.New.1]()
			- [github.com/PritOriginal/problem-map-server/internal/handler/marks.Register.func1.2.(*handler).GetMarkTypes.2]()

</details>
<details>
<summary>`/marks/user/{userId}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/marks**
	- **/user/{userId}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/marks.Register.func1.(*handler).GetMarksByUserId.5]()

</details>
<details>
<summary>`/marks/{id}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/marks**
	- **/{id}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/marks.Register.func1.(*handler).GetMarkById.4]()

</details>
<details>
<summary>`/swagger`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/swagger**
	- _GET_
		- [github.com/swaggo/http-swagger.Handler.func1]()

</details>
<details>
<summary>`/tasks`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/tasks**
	- **/**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/tasks.Register.func1.(*handler).GetTasks.1]()
		- _POST_
			- [github.com/PritOriginal/problem-map-server/internal/handler/tasks.Register.func1.(*handler).AddTask.4]()

</details>
<details>
<summary>`/tasks/user/{id}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/tasks**
	- **/user/{id}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/tasks.Register.func1.(*handler).GetTasksByUserId.3]()

</details>
<details>
<summary>`/tasks/{id}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/tasks**
	- **/{id}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/tasks.Register.func1.(*handler).GetTaskById.2]()

</details>
<details>
<summary>`/users`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/users**
	- **/**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/users.Register.func1.(*handler).GetUsers.1]()

</details>
<details>
<summary>`/users/{id}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/users**
	- **/{id}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/users.Register.func1.(*handler).GetUserById.2]()

</details>

Total # of routes: 21
