module Grid exposing (Grid(..), insert, fromGraph)

import Graph
import IntDict

type Grid n e
  = Cell (Graph.NodeContext n e)
  | Serial (Grid n e) (Grid n e)
  | Parallel (List (Grid n e))
  | End

fromGraph : Graph.Graph n e -> Grid n e
fromGraph graph =
  List.foldl insert End <|
    List.concat (Graph.heightLevels graph)

insert : Graph.NodeContext n e -> Grid n e -> Grid n e
insert nc grid =
  case IntDict.size nc.incoming of
    0 ->
      addToStart nc grid

    _ ->
      addAfterUpstreams nc grid

addToStart : Graph.NodeContext n e -> Grid n e -> Grid n e
addToStart nc graph =
  case graph of
    End ->
      Cell nc

    Parallel grids ->
      Parallel (Cell nc :: grids)

    _ ->
      Parallel
        [ graph
        , Cell nc
        ]

addAfterUpstreams : Graph.NodeContext n e -> Grid n e -> Grid n e
addAfterUpstreams nc grid =
  case grid of
    End ->
      End

    Parallel grids ->
      let
        (dependent, rest) =
          List.partition (leadsTo nc) grids
      in
        case dependent of
          [] ->
            grid

          [singlePath] ->
            Parallel (addAfterUpstreams nc singlePath :: rest)

          _ ->
            Parallel (addAfterMixedUpstreamsAndReinsertExclusiveOnes nc dependent :: rest)

    Serial a b ->
      if leadsTo nc a then
        Serial a (addToStart nc b)
      else
        Serial a (addAfterUpstreams nc b)

    Cell upstreamOrUnrelated ->
      if IntDict.member nc.node.id upstreamOrUnrelated.outgoing then
        Serial grid (Cell nc)
      else
        grid

addAfterMixedUpstreamsAndReinsertExclusiveOnes : Graph.NodeContext n e -> List (Grid n e) -> Grid n e
addAfterMixedUpstreamsAndReinsertExclusiveOnes nc dependent =
  let
    (exclusive, mixed) =
      List.partition (leadsOnlyTo nc) dependent

    loneUpstreams =
      List.concatMap nodes exclusive
  in
    case mixed of
      [single] ->
        List.foldr
          addBeforeDownstream
          (addAfterUpstreams nc single)
          loneUpstreams

      _ ->
        Serial (Parallel dependent) (Cell nc)

addBeforeDownstream : Graph.NodeContext n e -> Grid n e -> Grid n e
addBeforeDownstream nc grid =
  case grid of
    End ->
      End

    Parallel grids ->
      if comesFrom nc grid then
        Debug.crash "too late to add in front of Parallel"
      else
        Parallel (List.map (addBeforeDownstream nc) grids)

    Serial a b ->
      if comesFrom nc b then
        Serial (addToStart nc a) b
      else
        Serial a (addBeforeDownstream nc b)

    Cell upstreamOrUnrelated ->
      if comesFrom nc grid then
        Debug.crash "too late to add in front of Cell"
      else
        grid

leadsTo : Graph.NodeContext n e -> Grid n e -> Bool
leadsTo nc grid =
  case grid of
    End ->
      False

    Parallel grids ->
      List.any (leadsTo nc) grids

    Serial a b ->
      leadsTo nc a || leadsTo nc b

    Cell upstreamOrUnrelated ->
      IntDict.member nc.node.id upstreamOrUnrelated.outgoing

comesFrom : Graph.NodeContext n e -> Grid n e -> Bool
comesFrom nc grid =
  case grid of
    End ->
      False

    Parallel grids ->
      List.any (comesFrom nc) grids

    Serial a _ ->
      comesFrom nc a

    Cell upstreamOrUnrelated ->
      IntDict.member nc.node.id upstreamOrUnrelated.incoming

leadsOnlyTo : Graph.NodeContext n e -> Grid n e -> Bool
leadsOnlyTo nc grid =
  case grid of
    End ->
      False

    Parallel grids ->
      List.all (leadsOnlyTo nc) grids

    Serial _ b ->
      leadsOnlyTo nc b

    Cell upstreamOrUnrelated ->
      case IntDict.size upstreamOrUnrelated.outgoing of
        1 ->
          IntDict.member nc.node.id upstreamOrUnrelated.outgoing

        0 ->
          -- TODO: is it possible to have a Parallel with both an input and an orphaned job?
          -- Debug.crash "possible?"
          False

        _ ->
          False

flatten : Grid n e -> Grid n e
flatten grid =
  grid

nodes : Grid n e -> List (Graph.NodeContext n e)
nodes grid =
  case grid of
    End ->
      []

    Cell nc ->
      [nc]

    Parallel grids ->
      List.concatMap nodes grids

    Serial a b ->
      nodes a ++ nodes b
