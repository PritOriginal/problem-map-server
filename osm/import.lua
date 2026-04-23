local admin_boundaries = osm2pgsql.define_table({
    name = 'admin_boundaries',
    columns = {
        { column = 'id', sql_type = 'serial', create_only = true },
        { column = 'osm_id', type = 'bigint' },
        { column = 'name', type = 'text' },
        { column = 'admin_level', type = 'int2' },
        { column = 'tags', type = 'jsonb' },
        { column = 'geom', type = 'multipolygon', projection = 4326 }
    }
})

function osm2pgsql.process_relation(object)
    if object.tags.boundary == 'administrative' then
        local admin_level = object.tags.admin_level
        
        if admin_level then
            local geom = object:as_multipolygon()
            
            if geom then
                admin_boundaries:insert({
                    osm_id = object.id,
                    name = object.tags.name,
                    admin_level = admin_level,
                    tags = object.tags,
                    geom = geom
                })
            end
        end
    end
end