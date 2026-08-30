local point_table = osm2pgsql.define_table({
    name = 'my_points',
    ids = { type = 'node', id_column = 'osm_id' },
    columns = {
        { column = 'tags', type = 'jsonb' },
        { column = 'name', type = 'text' },
        { column = 'geom', type = 'point' }
    }
})

local polygon_table = osm2pgsql.define_table({
    name = 'my_polygons',
    ids = { type = 'area', id_column = 'osm_id' },
    columns = {
        { column = 'name', type = 'text' },
        { column = 'admin_level', type = 'text' },
        { column = 'tags', type = 'jsonb' },
        { column = 'geom', type = 'multipolygon' }
    }
})

function osm2pgsql.process_node(object)
    if object.tags.name then
        point_table:insert({
            osm_id = object.id,
            tags = object.tags,
            name = object.tags.name,
            geom = object:as_point()
        })
    end
end

function osm2pgsql.process_way(object)
    if object.is_closed and object.tags.name then
        polygon_table:insert({
            osm_id = object.id,
            name = object.tags.name,
            admin_level = object.tags.admin_level,
            tags = object.tags,
            geom = object:as_multipolygon()
        })
    end
end

-- function osm2pgsql.process_relation(object)
--     if object.tags.type == 'multipolygon' and object.tags.name then
--         polygon_table:insert({
--             osm_id = object.id,
--             name = object.tags.name,
--             admin_level = object.tags.admin_level,
--             tags = object.tags,
--             geom = object:as_multipolygon()
--         })
--     end
-- end

local districts_table = osm2pgsql.define_table({
    name = 'districts',
    columns = {
        { column = 'district_id', sql_type = 'serial', create_only = true },
        { column = 'admin_level', type = 'text' },
        { column = 'city_id', type = 'int' },
        { column = 'name', type = 'text' },
        { column = 'geom', type = 'multipolygon' }
    }
})

function osm2pgsql.process_relation(object)
    if object.tags.boundary == 'administrative' then
        local admin_level = object.tags.admin_level
        
        -- if admin_level == '9' or admin_level == '10' then
        if object.tags.admin_level then
            local geom = object:as_multipolygon()
            
            if geom then
                districts_table:insert({
                    admin_level = admin_level,
                    name = object.tags.name,
                    geom = geom
                })
            end
        end
    end
end