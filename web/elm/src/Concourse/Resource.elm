module Concourse.Resource exposing (fetchResourceVersions)

import Concourse
import Htt
import Json.Decode
import Task exposing (Task)

fetchResourceVersions : ResourceIdentifier -> Task Http.Error (List Concourse.ResourceVersion)
fetchResourceVersions rid =
  Http.get (Json.Decode.list Concourse.decodeResourceVersion) <|
    "/api/v1/teams/" ++ rid.teamName ++ "/pipelines/" ++ rid.pipelineName ++
    "/resources/" ++ rid.resourceName ++ "/versions"
